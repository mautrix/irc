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

package ircdb

import (
	"context"
	"embed"
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"regexp"
	"slices"
	"strconv"
	"sync"

	"github.com/rs/zerolog"
	"go.mau.fi/util/dbutil"
	"go.mau.fi/util/random"

	"maunium.net/go/mautrix/id"
)

type IRCDB struct {
	*dbutil.Database

	identQuery       *dbutil.QueryHelper[*IdentData]
	cacheLock        sync.RWMutex
	mxidToIdentCache map[id.UserID]*IdentData
	identToMXIDCache map[string]*IdentData
}

var table dbutil.UpgradeTable

//go:embed *.sql
var upgrades embed.FS

func init() {
	table.RegisterFS(upgrades)
}

func New(db *dbutil.Database, log zerolog.Logger) *IRCDB {
	db = db.Child("irc_version", table, dbutil.ZeroLogger(log))
	return &IRCDB{
		Database: db,

		mxidToIdentCache: make(map[id.UserID]*IdentData),
		identToMXIDCache: make(map[string]*IdentData),
		identQuery: dbutil.MakeQueryHelper(db, func(_ *dbutil.QueryHelper[*IdentData]) *IdentData {
			return &IdentData{}
		}),
	}
}

func (db *IRCDB) FillCache(ctx context.Context) error {
	return db.identQuery.
		QueryManyIter(ctx, "SELECT rowid, mxid, ident FROM irc_ident").
		Iter(func(data *IdentData) (bool, error) {
			db.addIdentToCache(data)
			return true, nil
		})
}

var nonAlphanumRegex = regexp.MustCompile("[^a-zA-Z0-9]")

func (db *IRCDB) GetIdent(ctx context.Context, userID id.UserID) (string, error) {
	db.cacheLock.RLock()
	cached, ok := db.mxidToIdentCache[userID]
	db.cacheLock.RUnlock()
	if ok {
		return cached.Ident, nil
	}

	db.cacheLock.Lock()
	defer db.cacheLock.Unlock()
	cached, ok = db.mxidToIdentCache[userID]
	if ok {
		return cached.Ident, nil
	}
	localpart, homeserver, err := userID.Parse()
	if err != nil {
		return "", fmt.Errorf("failed to parse user ID: %w", err)
	}
	localpart = nonAlphanumRegex.ReplaceAllString(localpart, "")
	homeserver = nonAlphanumRegex.ReplaceAllString(homeserver, "")
	ident := localpart + homeserver
	if len(ident) > 10 {
		ident = ident[:10]
	} else if len(ident) < 4 {
		ident += random.String(5)
	}
	baseIdent := ident
	if len(baseIdent) > 7 {
		baseIdent = baseIdent[:7]
	}
	for i := 0; i < 36*36*36 && db.identExists(ident); i++ {
		ident = baseIdent + strconv.FormatUint(uint64(i), 36)
	}
	if db.identExists(ident) {
		return "", fmt.Errorf("too many conflicting idents already stored")
	}
	var rowid int
	err = db.QueryRow(
		ctx,
		"INSERT INTO irc_ident (mxid, ident) VALUES ($1, $2) RETURNING rowid",
		userID,
		ident,
	).Scan(&rowid)
	if err != nil {
		return "", fmt.Errorf("failed to insert ident mapping %q->%q: %w", userID, ident, err)
	}
	db.addIdentToCache(&IdentData{
		RowID:  rowid,
		UserID: userID,
		Ident:  ident,
	})
	return ident, nil
}

func (db *IRCDB) identExists(ident string) bool {
	_, exists := db.identToMXIDCache[ident]
	return exists
}

func (db *IRCDB) addIdentToCache(data *IdentData) {
	db.mxidToIdentCache[data.UserID] = data
	db.identToMXIDCache[data.Ident] = data
}

type IdentData struct {
	RowID  int
	UserID id.UserID
	Ident  string
}

func (id *IdentData) IPv6(mask net.IPNet) net.IP {
	if id.RowID <= 0 || len(mask.IP) != 16 {
		return mask.IP
	}
	ip := slices.Clone(mask.IP)
	baseVal := binary.BigEndian.Uint32(ip[12:16])
	if uint64(baseVal)+uint64(id.RowID) > math.MaxUint32 {
		return mask.IP
	}
	binary.BigEndian.PutUint32(ip[12:16], baseVal+uint32(id.RowID))
	if !mask.Contains(ip) {
		return mask.IP
	}
	return ip
}

func (id *IdentData) Scan(rows dbutil.Scannable) (*IdentData, error) {
	err := rows.Scan(&id.RowID, &id.UserID, &id.Ident)
	if err != nil {
		return nil, err
	}
	return id, nil
}
