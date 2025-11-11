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
	"fmt"
	"strconv"
	"strings"

	"go.mau.fi/util/exslices"
	"golang.org/x/net/html/atom"
	"maunium.net/go/mautrix/event"
)

func ASCIIToContent(text string) *event.MessageEventContent {
	converted := ParseASCII(text)
	if converted == "" || converted == event.TextToHTML(text) {
		return &event.MessageEventContent{
			MsgType: event.MsgText,
			Body:    text,
		}
	}
	return &event.MessageEventContent{
		MsgType:       event.MsgText,
		Body:          StripASCII(text),
		Format:        event.FormatHTML,
		FormattedBody: converted,
	}
}

func ParseASCII(text string) string {
	var out strings.Builder
	var tagStack exslices.Stack[atom.Atom]
	var currentFg, currentBg string
	updateColors := func(newFg, newBg string) {
		if currentFg != "" || currentBg != "" {
			var readdTags []atom.Atom
			for tag := range tagStack.PopIter() {
				_, _ = fmt.Fprintf(&out, "</%s>", tag)
				if tag == atom.Span {
					break
				} else {
					readdTags = append(readdTags, tag)
				}
			}
			for _, tag := range readdTags {
				_, _ = fmt.Fprintf(&out, "<%s>", tag)
				tagStack.Push(tag)
			}
		}
		if newFg != "" || newBg != "" {
			if newFg == newBg {
				out.WriteString("<span data-mx-spoiler>")
			} else {
				_, _ = fmt.Fprintf(&out, `<span data-mx-color="%s" data-mx-bg-color="%s">`, newFg, newBg)
			}
			tagStack.Push(atom.Span)
		}
		currentFg, currentBg = newFg, newBg
	}
	doReset := func() {
		for tag := range tagStack.PopIter() {
			_, _ = fmt.Fprintf(&out, "</%s>", tag)
		}
	}
	for {
		nextIdx := strings.IndexAny(text, metacharacters)
		if nextIdx == -1 {
			break
		}
		out.WriteString(event.TextToHTML(text[:nextIdx]))
		meta := text[nextIdx]
		text = text[nextIdx+1:]
		switch meta {
		case bold[0]:
			out.WriteString("<strong>")
			tagStack.Push(atom.Strong)
		case monospace[0]:
			out.WriteString("<code>")
			tagStack.Push(atom.Code)
		case italic[0]:
			out.WriteString("<em>")
			tagStack.Push(atom.Em)
		case strikethrough[0]:
			out.WriteString("<del>")
			tagStack.Push(atom.Del)
		case underline[0]:
			out.WriteString("<u>")
			tagStack.Push(atom.U)
		case color[0]:
			newFg, newBg, rest := parseColor(text, currentBg)
			updateColors(newFg, newBg)
			text = rest
		case hexColor[0]:
			newFg, rest := parseHexColor(text)
			updateColors(newFg, currentBg)
			text = rest
		case reverseColour[0]:
			updateColors(currentBg, currentFg)
		case reset[0]:
			doReset()
		default:
			panic(fmt.Errorf("impossible case: IndexAny(metacharacters) returned %c", meta))
		}
	}
	if out.Len() == 0 {
		return ""
	}
	out.WriteString(event.TextToHTML(text))
	doReset()
	return out.String()
}

func StripASCII(text string) string {
	var out strings.Builder
	for {
		nextIdx := strings.IndexAny(text, metacharacters)
		if nextIdx == -1 {
			break
		}
		out.WriteString(text[:nextIdx])
		meta := text[nextIdx]
		text = text[nextIdx+1:]
		switch meta {
		case color[0]:
			_, _, text = parseColor(text, "")
		case hexColor[0]:
			_, text = parseHexColor(text)
		}
	}
	if out.Len() == 0 {
		return text
	}
	out.WriteString(text)
	return out.String()
}

func parseColor(text, currentBg string) (fg, bg, rest string) {
	submatch := colorRegex.FindStringSubmatch(text)
	if submatch == nil {
		return "", "", text
	}
	fgNum, err := strconv.Atoi(submatch[1])
	if err != nil {
		return "", "", text
	}
	fg = colors[fgNum]
	if submatch[2] != "" {
		bgNum, err := strconv.Atoi(submatch[2])
		if err != nil {
			return "", "", text
		}
		bg = colors[bgNum]
	} else {
		bg = currentBg
	}
	rest = text[len(submatch[0]):]
	return
}

func isHex(text string) bool {
	for _, c := range text {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') && (c < 'A' || c > 'F') {
			return false
		}
	}
	return true
}

func parseHexColor(text string) (fg, rest string) {
	if len(text) < 6 {
		return "", text
	}
	hexCode := text[:6]
	if !isHex(hexCode) {
		return "", text
	}
	fg = "#" + hexCode
	rest = text[6:]
	return
}
