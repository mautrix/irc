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
	"fmt"
)

type ISupport struct {
	ChanTypes  string
	PLPrefixes map[byte]int
}

var defaultISupport = ParseISupport(map[string]string{})

func modeLetterToPowerLevel(letter byte) int {
	switch letter {
	case 'q': // owner, founder
		return 95
	case 'a': // admin, protected
		return 75
	case 'o': // operator
		return 50
	case 'h': // half-op
		return 45
	case 'v': // voice
		return 1
	default:
		return 0
	}
}

func ParseISupport(raw map[string]string) *ISupport {
	isupport := &ISupport{
		PLPrefixes: make(map[byte]int),
	}
	if ct, ok := raw["CHANTYPES"]; ok {
		isupport.ChanTypes = ct
	} else {
		isupport.ChanTypes = "#"
	}
	if prefixes, ok := raw["PREFIX"]; ok {
		var modes, symbols string
		_, err := fmt.Sscanf(prefixes, "(%s)%s", &modes, &symbols)
		if err != nil || modes == "" || symbols == "" || len(modes) != len(symbols) {
			modes = "qaohv"
			symbols = "~&@%+"
		}
		for i := 0; i < len(modes); i++ {
			isupport.PLPrefixes[symbols[i]] = modeLetterToPowerLevel(modes[i])
		}
	}
	return isupport
}
