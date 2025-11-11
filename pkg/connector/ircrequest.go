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

package connector

import (
	"context"
	"fmt"

	"github.com/ergochat/irc-go/ircevent"
	"github.com/ergochat/irc-go/ircmsg"
	"github.com/pkg/errors"
)

func (ic *IRCClient) sendAndWaitResponse(ctx context.Context, tags map[string]string, cmd string, args ...string) (*ircevent.Batch, error) {
	resp, err := ic.Conn.GetLabeledResponse(tags, cmd, args...)
	if errors.Is(err, ircevent.CapabilityNotNegotiated) {
		return ic.sendAndWaitResponseFallback(ctx, tags, cmd, args...)
	}
	return resp, err
}

func (ic *IRCClient) sendAndWaitResponseFallback(ctx context.Context, tags map[string]string, cmd string, args ...string) (*ircevent.Batch, error) {
	return nil, fmt.Errorf("labeled responses not supported by server")
	//ch := make(chan *ircmsg.Message, 1)
	//if labeledResponse {
	//	ic.sendWaitersLock.Lock()
	//	ic.sendWaiters[label] = sendWaiter{ch: ch}
	//	ic.sendWaitersLock.Unlock()
	//} else {
	//	// TODO wait based on response numerics?
	//	ch <- nil
	//}
	//err := ic.Conn.SendIRCMessage(wrapped)
	//if err != nil {
	//	return nil, err
	//}
	//select {
	//case <-ctx.Done():
	//	return nil, ctx.Err()
	//case resp := <-ch:
	//	return resp, nil
	//}
}

func (ic *IRCClient) onDisconnect(message ircmsg.Message) {
	ic.sendWaitersLock.Lock()
	defer ic.sendWaitersLock.Unlock()
	for _, waiter := range ic.sendWaiters {
		close(waiter.ch)
	}
	clear(ic.sendWaiters)
}

func (ic *IRCClient) onPotentialEchoMessage(msg ircmsg.Message) bool {
	if msg.Nick() != ic.Conn.CurrentNick() {
		return false
	}
	ic.sendWaitersLock.Lock()
	waiter, ok := ic.sendWaiters[msg.Params[0]]
	ok = ok && msg.Command == waiter.cmd
	if ok {
		delete(ic.sendWaiters, msg.Params[0])
	}
	ic.sendWaitersLock.Unlock()
	if ok {
		waiter.ch <- &msg
		return true
	}
	return false
}

func (ic *IRCClient) sendAndWaitMessage(ctx context.Context, tags map[string]string, waiterCmd, cmd string, args ...string) (*ircmsg.Message, error) {
	channel := args[0]
	labelResp, err := ic.Conn.GetLabeledResponse(tags, cmd, args...)
	if err != nil && !errors.Is(err, ircevent.CapabilityNotNegotiated) {
		return nil, err
	} else if labelResp != nil {
		if labelResp.Command == "BATCH" {
			// TODO check that it's a multiline batch?
			return multilineBatchToMessage(labelResp), nil
		}
		return &labelResp.Message, nil
	}
	if waiterCmd == "" {
		waiterCmd = cmd
	}
	wrapped := ircmsg.MakeMessage(tags, "", cmd, args...)
	_, willEcho := ic.Conn.AcknowledgedCaps()["echo-message"]
	ch := make(chan *ircmsg.Message, 1)
	if willEcho {
		ic.sendWaitersLock.Lock()
		ic.sendWaiters[channel] = sendWaiter{ch: ch, cmd: waiterCmd}
		ic.sendWaitersLock.Unlock()
	} else {
		ch <- &wrapped
	}
	err = ic.Conn.SendIRCMessage(wrapped)
	if err != nil {
		return nil, err
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-ch:
		if resp == nil {
			return nil, fmt.Errorf("no echo received")
		}
		return resp, nil
	}
}
