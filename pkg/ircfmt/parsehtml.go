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

package ircfmt

import (
	"context"
	"strings"

	"maunium.net/go/mautrix/event"
	"maunium.net/go/mautrix/format"
)

var htmlParser = format.HTMLParser{
	TabsToSpaces:           4,
	Newline:                "\n",
	HorizontalLine:         "---",
	BoldConverter:          formattingAdder(bold),
	ItalicConverter:        formattingAdder(italic),
	StrikethroughConverter: formattingAdder(strikethrough),
	UnderlineConverter:     formattingAdder(underline),
	SpoilerConverter: func(text, reason string, ctx format.Context) string {
		return doAddFormatting(text, color+"01,01")
	},
	ColorConverter: func(text, fg, bg string, ctx format.Context) string {
		ircFG := reverseColors[strings.ToLower(fg)]
		ircBG := reverseColors[strings.ToLower(bg)]
		var resultFmt string
		if ircFG != "" {
			resultFmt = color + ircFG
			if ircBG != "" {
				resultFmt += "," + ircBG
			}
		}
		if len(fg) == 7 && fg[0] == '#' && isHex(fg[1:]) {
			resultFmt = hexColor + strings.ToUpper(fg[1:])
		}
		if resultFmt == "" {
			return text
		}
		return doAddFormatting(text, resultFmt)
	},
	MonospaceBlockConverter: func(code, language string, ctx format.Context) string {
		return doAddFormatting(code, monospace)
	},
	MonospaceConverter: formattingAdder(monospace),
	TextConverter: func(s string, context format.Context) string {
		return StripASCII(s)
	},
}

func doAddFormatting(s, fmt string) string {
	return fmt + strings.ReplaceAll(strings.TrimRight(s, reset), reset, reset+fmt) + reset
}

func formattingAdder(fmt string) func(s string, context format.Context) string {
	return func(s string, context format.Context) string {
		return doAddFormatting(s, fmt)
	}
}

func ContentToASCII(ctx context.Context, content *event.MessageEventContent) string {
	if content.MsgType.IsMedia() && content.Body == content.GetFileName() {
		return ""
	} else if content.Format != event.FormatHTML {
		return StripASCII(content.Body)
	}
	return ParseHTML(ctx, content.FormattedBody)
}

func ParseHTML(ctx context.Context, html string) string {
	return htmlParser.Parse(html, format.NewContext(ctx))
}
