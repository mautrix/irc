package ircfmt

import (
	"regexp"
	"strconv"
)

const (
	// raw bytes and strings to do replacing with
	bold          = "\x02"
	color         = "\x03"
	hexColor      = "\x04"
	monospace     = "\x11"
	reverseColour = "\x16"
	italic        = "\x1d"
	strikethrough = "\x1e"
	underline     = "\x1f"
	reset         = "\x0f"

	metacharacters = bold + color + hexColor + monospace + reverseColour + italic + strikethrough + underline + reset
)

var colorRegex = regexp.MustCompile(`^([0-9]{1,2})(?:,([0-9]{1,2}))?`)

var colors = [100]string{
	"#ffffff", // 00 white
	"#000000", // 01 black
	"#00007f", // 02 blue (navy)
	"#009300", // 03 green
	"#ff0000", // 04 red
	"#7f0000", // 05 brown (maroon)
	"#9c009c", // 06 purple
	"#fc7f00", // 07 orange (olive)
	"#ffff00", // 08 yellow
	"#00fc00", // 09 light green (lime)
	"#009393", // 10 teal (cyan)
	"#00ffff", // 11 light cyan (aqua)
	"#0000fc", // 12 light blue (royal)
	"#ff00ff", // 13 pink (fuchsia)
	"#7f7f7f", // 14 grey
	"#d2d2d2", // 15 light grey (silver)
	"#470000", // 16
	"#472100", // 17
	"#474700", // 18
	"#324700", // 19
	"#004700", // 20
	"#00472c", // 21
	"#004747", // 22
	"#002747", // 23
	"#000047", // 24
	"#2e0047", // 25
	"#470047", // 26
	"#47002a", // 27
	"#740000", // 28
	"#743a00", // 29
	"#747400", // 30
	"#517400", // 31
	"#007400", // 32
	"#007449", // 33
	"#007474", // 34
	"#004074", // 35
	"#000074", // 36
	"#4b0074", // 37
	"#740074", // 38
	"#740045", // 39
	"#b50000", // 40
	"#b56300", // 41
	"#b5b500", // 42
	"#7db500", // 43
	"#00b500", // 44
	"#00b571", // 45
	"#00b5b5", // 46
	"#0063b5", // 47
	"#0000b5", // 48
	"#7500b5", // 49
	"#b500b5", // 50
	"#b5006b", // 51
	"#ff0000", // 52
	"#ff8c00", // 53
	"#ffff00", // 54
	"#b2ff00", // 55
	"#00ff00", // 56
	"#00ffa0", // 57
	"#00ffff", // 58
	"#008cff", // 59
	"#0000ff", // 60
	"#a500ff", // 61
	"#ff00ff", // 62
	"#ff0098", // 63
	"#ff5959", // 64
	"#ffb459", // 65
	"#ffff71", // 66
	"#cfff60", // 67
	"#6fff6f", // 68
	"#65ffc9", // 69
	"#6dffff", // 70
	"#59b4ff", // 71
	"#5959ff", // 72
	"#c459ff", // 73
	"#ff66ff", // 74
	"#ff59bc", // 75
	"#ff9c9c", // 76
	"#ffd39c", // 77
	"#ffff9c", // 78
	"#e2ff9c", // 79
	"#9cff9c", // 80
	"#9cffdb", // 81
	"#9cffff", // 82
	"#9cd3ff", // 83
	"#9c9cff", // 84
	"#dc9cff", // 85
	"#ff9cff", // 86
	"#ff94d3", // 87
	"#000000", // 88
	"#131313", // 89
	"#282828", // 90
	"#363636", // 91
	"#4d4d4d", // 92
	"#656565", // 93
	"#818181", // 94
	"#9f9f9f", // 95
	"#bcbcbc", // 96
	"#e2e2e2", // 97
	"#ffffff", // 98
	"",        // 99 (default)
}

var reverseColors map[string]string

func init() {
	reverseColors = make(map[string]string, len(colors))
	for i, col := range colors {
		idxStr := strconv.Itoa(i)
		if len(idxStr) == 1 {
			idxStr = "0" + idxStr
		}
		reverseColors[idxStr] = col
	}
}
