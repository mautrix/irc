// mautrix-irc - A Matrix-IRC puppeting bridge.
// Copyright (C) 2025 Tulir Asokan
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package identd

import (
	"bufio"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
)

type identTuple struct {
	localPort  int
	remotePort int
	remoteAddr string
}

func (it identTuple) MarshalZerologObject(e *zerolog.Event) {
	e.Int("local_port", it.localPort)
	e.Int("remote_port", it.remotePort)
	if it.remoteAddr != "" {
		e.Str("remote_addr", it.remoteAddr)
	}
}

type Identd struct {
	log   zerolog.Logger
	addr  string
	ports map[identTuple]string
	lock  sync.RWMutex
	serv  atomic.Pointer[net.Listener]

	strictRemote bool
}

func NewIdentd(log zerolog.Logger, addr string, strictRemote bool) *Identd {
	return &Identd{
		log:          log,
		addr:         addr,
		strictRemote: strictRemote,
		ports:        make(map[identTuple]string),
	}
}

func noop() {}

func (id *Identd) Add(local, remote, username string) func() {
	localParts := strings.Split(local, ":")
	remoteParts := strings.Split(remote, ":")
	if len(localParts) < 2 || len(remoteParts) < 2 {
		id.log.Warn().
			Str("local_address", local).
			Str("remote_address", remote).
			Str("username", username).
			Msg("Unexpected parameters to Identd.Add")
		return noop
	}
	localPort, err := strconv.Atoi(localParts[len(localParts)-1])
	if err != nil || localPort <= 0 || localPort > 65535 {
		id.log.Warn().
			Str("local_address", local).
			Str("remote_address", remote).
			Str("username", username).
			Msg("Unexpected parameters to Identd.Add: bad local port")
		return noop
	}
	remotePort, err := strconv.Atoi(remoteParts[len(remoteParts)-1])
	if err != nil || remotePort <= 0 || remotePort > 65535 {
		id.log.Warn().
			Str("local_address", local).
			Str("remote_address", remote).
			Str("username", username).
			Msg("Unexpected parameters to Identd.Add: bad remote port")
		return noop
	}
	tuple := identTuple{
		localPort:  localPort,
		remotePort: remotePort,
		remoteAddr: strings.Join(remoteParts[:len(remoteParts)-1], ":"),
	}
	if !id.strictRemote {
		tuple.remoteAddr = ""
	}
	id.lock.Lock()
	id.ports[tuple] = username
	id.lock.Unlock()
	id.log.Debug().Any("key", tuple).Str("username", username).Msg("Added identd entry")
	return func() {
		removed := false
		id.lock.Lock()
		if id.ports[tuple] == username {
			delete(id.ports, tuple)
			removed = true
		}
		id.lock.Unlock()
		if removed {
			id.log.Debug().Any("key", tuple).Str("username", username).Msg("Removed identd entry")
		}
	}
}

func (id *Identd) Start() error {
	if id.addr == "" {
		return nil
	}
	ln, err := net.Listen("tcp", id.addr)
	if err != nil {
		return err
	}
	id.log.Info().Str("address", id.addr).Msg("Identd server started")
	id.serv.Store(&ln)
	go id.acceptConns()
	return nil
}

func (id *Identd) Stop() error {
	serv := id.serv.Swap(nil)
	if serv != nil {
		return (*serv).Close()
	}
	return nil
}

func (id *Identd) acceptConns() {
	servPtr := id.serv.Load()
	if servPtr == nil {
		return
	}
	ln := *servPtr
	for servPtr == id.serv.Load() {
		conn, err := ln.Accept()
		if err != nil {
			logEvt := id.log.Err(err)
			if conn != nil {
				logEvt.Stringer("remote_addr", conn.RemoteAddr())
			}
			logEvt.Msg("Failed to accept connection")
			continue
		}
		id.log.Trace().Stringer("remote_addr", conn.RemoteAddr()).Msg("Accepted identd connection")
		go id.handleConn(conn)
	}
	id.log.Info().Msg("Identd server stopped")
}

func (id *Identd) handleConn(conn net.Conn) {
	defer func() {
		_ = conn.Close()
	}()
	logEvt := id.log.Debug().Stringer("remote_addr", conn.RemoteAddr())
	br := bufio.NewReaderSize(conn, 512)
	msg, err := br.ReadString('\n')
	if err != nil {
		logEvt.Err(err).Msg("Failed to read identd request")
		return
	}
	reqParts := strings.Split(strings.TrimSpace(msg), ",")
	if len(reqParts) != 2 {
		logEvt.Str("raw_payload", msg).Msg("Malformed identd request: incorrect number of parts")
		return
	}
	localPort, err := strconv.Atoi(strings.TrimSpace(reqParts[0]))
	if err != nil {
		logEvt.Str("raw_payload", msg).Msg("Malformed identd request: local port is not a number")
		return
	}
	remotePort, err := strconv.Atoi(strings.TrimSpace(reqParts[1]))
	if err != nil {
		logEvt.Str("raw_payload", msg).Msg("Malformed identd request: remote port is not a number")
		return
	}
	addrStr := conn.RemoteAddr().String()
	portIdx := strings.LastIndexByte(addrStr, ':')
	if portIdx == -1 {
		logEvt.Str("raw_payload", msg).Msg("Malformed identd request: remote address has no port")
		return
	}
	tuple := identTuple{
		localPort:  localPort,
		remotePort: remotePort,
		remoteAddr: addrStr[:portIdx],
	}
	if !id.strictRemote {
		tuple.remoteAddr = ""
	}
	id.lock.RLock()
	username, ok := id.ports[tuple]
	id.lock.RUnlock()
	var writeErr error
	if !ok {
		_, writeErr = fmt.Fprintf(conn, "%d, %d : ERROR : NO-USER\r\n", localPort, remotePort)
	} else {
		_, writeErr = fmt.Fprintf(conn, "%d, %d : USERID : UNIX : %s\r\n", localPort, remotePort, username)
	}
	if writeErr != nil {
		logEvt.AnErr("write_error", writeErr)
	}
	logEvt.Object("key", tuple).Str("username", username).Bool("ok", ok).Msg("Handled identd request")
}
