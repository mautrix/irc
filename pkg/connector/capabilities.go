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

	"go.mau.fi/util/ptr"
	"maunium.net/go/mautrix/bridgev2"
	"maunium.net/go/mautrix/event"
)

func (ic *IRCConnector) GetBridgeInfoVersion() (info, capabilities int) {
	return 1, 1
}

var genCaps = &bridgev2.NetworkGeneralCapabilities{
	Provisioning: bridgev2.ProvisioningCapabilities{
		ResolveIdentifier: bridgev2.ResolveIdentifierCapabilities{
			CreateDM:       true,
			LookupUsername: true,
			ContactList:    false,
			Search:         false,
		},
		GroupCreation: nil,
	},
}

func (ic *IRCConnector) GetCapabilities() *bridgev2.NetworkGeneralCapabilities {
	return genCaps
}

var caps = &event.RoomFeatures{
	ID: "fi.mau.irc.capabilities.2025_11_10",
	File: map[event.CapabilityMsgType]*event.FileFeatures{
		event.MsgImage: {
			MimeTypes:        map[string]event.CapabilitySupportLevel{"*/*": event.CapLevelFullySupported},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: 400,
		},
		event.MsgAudio: {
			MimeTypes:        map[string]event.CapabilitySupportLevel{"*/*": event.CapLevelFullySupported},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: 400,
		},
		event.MsgVideo: {
			MimeTypes:        map[string]event.CapabilitySupportLevel{"*/*": event.CapLevelFullySupported},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: 400,
		},
		event.MsgFile: {
			MimeTypes:        map[string]event.CapabilitySupportLevel{"*/*": event.CapLevelFullySupported},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: 400,
		},
		event.CapMsgVoice: {
			MimeTypes:        map[string]event.CapabilitySupportLevel{"*/*": event.CapLevelFullySupported},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: 400,
		},
		event.CapMsgSticker: {
			MimeTypes:        map[string]event.CapabilitySupportLevel{"*/*": event.CapLevelFullySupported},
			Caption:          event.CapLevelFullySupported,
			MaxCaptionLength: 400,
		},
	},
	Reply:         event.CapLevelPartialSupport,
	MaxTextLength: 500,
}

var relayCaps = ptr.Clone(caps)

func init() {
	relayCaps.PerMessageProfileRelay = true
}

func (ic *IRCClient) GetCapabilities(ctx context.Context, portal *bridgev2.Portal) *event.RoomFeatures {
	_, canRelay := ic.Conn.AcknowledgedCaps()["draft/relaymsg"]
	if canRelay {
		return relayCaps
	}
	return caps
}
