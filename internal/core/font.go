package core

import "strings"

// A 5x5 block font for banners. Each glyph is 5 rows of 5 columns, '#' = on.
var font = map[byte][5]string{
	'A': {" ### ", "#   #", "#####", "#   #", "#   #"},
	'B': {"#### ", "#   #", "#### ", "#   #", "#### "},
	'C': {" ####", "#    ", "#    ", "#    ", " ####"},
	'D': {"#### ", "#   #", "#   #", "#   #", "#### "},
	'E': {"#####", "#    ", "#### ", "#    ", "#####"},
	'F': {"#####", "#    ", "#### ", "#    ", "#    "},
	'G': {" ####", "#    ", "#  ##", "#   #", " ####"},
	'H': {"#   #", "#   #", "#####", "#   #", "#   #"},
	'I': {"#####", "  #  ", "  #  ", "  #  ", "#####"},
	'J': {"  ###", "    #", "    #", "#   #", " ### "},
	'K': {"#   #", "#  # ", "###  ", "#  # ", "#   #"},
	'L': {"#    ", "#    ", "#    ", "#    ", "#####"},
	'M': {"#   #", "## ##", "# # #", "#   #", "#   #"},
	'N': {"#   #", "##  #", "# # #", "#  ##", "#   #"},
	'O': {" ### ", "#   #", "#   #", "#   #", " ### "},
	'P': {"#### ", "#   #", "#### ", "#    ", "#    "},
	'Q': {" ### ", "#   #", "# # #", "#  # ", " ## #"},
	'R': {"#### ", "#   #", "#### ", "#  # ", "#   #"},
	'S': {" ####", "#    ", " ### ", "    #", "#### "},
	'T': {"#####", "  #  ", "  #  ", "  #  ", "  #  "},
	'U': {"#   #", "#   #", "#   #", "#   #", " ### "},
	'V': {"#   #", "#   #", "#   #", " # # ", "  #  "},
	'W': {"#   #", "#   #", "# # #", "## ##", "#   #"},
	'X': {"#   #", " # # ", "  #  ", " # # ", "#   #"},
	'Y': {"#   #", " # # ", "  #  ", "  #  ", "  #  "},
	'Z': {"#####", "   # ", "  #  ", " #   ", "#####"},
	'0': {" ### ", "#  ##", "# # #", "##  #", " ### "},
	'1': {"  #  ", " ##  ", "  #  ", "  #  ", " ### "},
	'2': {" ### ", "#   #", "  ## ", " #   ", "#####"},
	'3': {"#### ", "    #", " ### ", "    #", "#### "},
	'4': {"#   #", "#   #", "#####", "    #", "    #"},
	'5': {"#####", "#    ", "#### ", "    #", "#### "},
	'6': {" ### ", "#    ", "#### ", "#   #", " ### "},
	'7': {"#####", "    #", "   # ", "  #  ", "  #  "},
	'8': {" ### ", "#   #", " ### ", "#   #", " ### "},
	'9': {" ### ", "#   #", " ####", "    #", " ### "},
	'!': {"  #  ", "  #  ", "  #  ", "     ", "  #  "},
	'?': {" ### ", "#   #", "  ## ", "     ", "  #  "},
	'-': {"     ", "     ", " ### ", "     ", "     "},
	'%': {"##  #", "## # ", "  #  ", " # ##", "#  ##"},
	'.': {"     ", "     ", "     ", "     ", "  #  "},
	' ': {"     ", "     ", "     ", "     ", "     "},
}

// BannerSize returns the cell size of text drawn at scale s: each font pixel
// becomes s columns by ceil(s/2) rows, so glyphs stay square on screen.
func BannerSize(text string, s int) (w, h int) {
	n := len(text)
	if n == 0 {
		return 0, 0
	}
	return (n*6 - 1) * s, 5 * ((s + 1) / 2)
}

// FitScale is the largest scale at which text fits in w x h.
func FitScale(text string, w, h int) int {
	s := 1
	for {
		bw, bh := BannerSize(text, s+1)
		if bw > w || bh > h {
			return s
		}
		s++
	}
}

// DrawBanner draws text in the block font with its top-left at (x0, y0),
// using fill as the character for every pixel.
func DrawBanner(v *View, text string, x0, y0, s int, fill byte, fg uint8) {
	text = strings.ToUpper(text)
	sy := (s + 1) / 2
	for i := 0; i < len(text); i++ {
		g, ok := font[text[i]]
		if !ok {
			g = font['?']
		}
		gx := x0 + i*6*s
		for row := 0; row < 5; row++ {
			for col := 0; col < 5; col++ {
				if g[row][col] != '#' {
					continue
				}
				for dy := 0; dy < sy; dy++ {
					for dx := 0; dx < s; dx++ {
						v.Set(gx+col*s+dx, y0+row*sy+dy, fill, fg)
					}
				}
			}
		}
	}
}
