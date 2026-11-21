package markdown

import (
	"sort"
	"strings"

	"github.com/yuin/goldmark/ast"
	extast "github.com/yuin/goldmark/extension/ast"
)

const (
	tableMinColWidth = 3

	tblH  = "─"
	tblV  = "│"
	tblTL = "┌"
	tblTR = "┐"
	tblBL = "└"
	tblBR = "┘"
	tblML = "├"
	tblMR = "┤"
	tblTM = "┬"
	tblBM = "┴"
	tblMM = "┼"
)

// renderTable emits a GFM table as Micron box drawing, matching the NomadNet
// MarkdownToMicron format_table output. Cell contents are already rendered
// Micron strings.
func (r *renderer) renderTable(n *extast.Table) {
	alignments := make([]string, len(n.Alignments))
	for i, a := range n.Alignments {
		alignments[i] = tableAlignment(a)
	}

	var header []string
	var rows [][]string
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		switch v := c.(type) {
		case *extast.TableHeader:
			header = r.tableCells(v)
		case *extast.TableRow:
			rows = append(rows, r.tableCells(v))
		}
	}
	if len(header) == 0 {
		return
	}
	for len(alignments) < len(header) {
		alignments = append(alignments, "left")
	}
	alignments = alignments[:len(header)]
	for i := range rows {
		for len(rows[i]) < len(header) {
			rows[i] = append(rows[i], "")
		}
		rows[i] = rows[i][:len(header)]
	}

	lines := formatTable(header, rows, alignments, r.maxWidth, r.opts.TableAlign)
	r.lines = append(r.lines, lines...)
}

func tableAlignment(a extast.Alignment) string {
	switch a {
	case extast.AlignCenter:
		return "center"
	case extast.AlignRight:
		return "right"
	default:
		return "left"
	}
}

// tableCells renders the inline content of each cell in a table row.
func (r *renderer) tableCells(row ast.Node) []string {
	var cells []string
	for c := row.FirstChild(); c != nil; c = c.NextSibling() {
		cell, ok := c.(*extast.TableCell)
		if !ok {
			continue
		}
		text := strings.ReplaceAll(r.renderInline(cell), "\n", " ")
		// Escape pipes so the cell survives pipe-row parsing, matching
		// _parse_table_row's backslash handling.
		text = strings.ReplaceAll(text, "|", "\\|")
		cells = append(cells, text)
	}
	return cells
}

// formatTable renders header and data rows as a Micron box drawing table.
// Cells must already be Micron markup. align is the block alignment wrapper,
// one of "", "l", "c", "r" (or the words left, center, right).
func formatTable(header []string, rows [][]string, alignments []string, maxWidth int, align string) []string {
	if len(header) == 0 {
		return nil
	}
	numCols := len(header)

	colWidths := make([]int, numCols)
	for i, cell := range header {
		colWidths[i] = max(colWidths[i], visibleWidth(cell))
	}
	for _, row := range rows {
		for i, cell := range row {
			if i < numCols {
				colWidths[i] = max(colWidths[i], visibleWidth(cell))
			}
		}
	}
	for i := range colWidths {
		colWidths[i] = max(colWidths[i], tableMinColWidth)
	}

	totalWidth := 0
	for _, w := range colWidths {
		totalWidth += w
	}
	totalWidth += numCols*3 + 1
	if totalWidth > maxWidth && maxWidth > 0 {
		excess := totalWidth - maxWidth
		idx := make([]int, numCols)
		for i := range idx {
			idx[i] = i
		}
		sort.SliceStable(idx, func(a, b int) bool { return colWidths[idx[a]] > colWidths[idx[b]] })
		for _, i := range idx {
			if excess <= 0 {
				break
			}
			reduction := min(excess, colWidths[i]-tableMinColWidth)
			if reduction < 0 {
				reduction = 0
			}
			colWidths[i] -= reduction
			excess -= reduction
		}
	}

	var out []string
	if a := tableAlignCode(align); a != "" {
		out = append(out, "`"+a)
	}

	var top strings.Builder
	top.WriteString(tblTL)
	for i, w := range colWidths {
		top.WriteString(strings.Repeat(tblH, w+2))
		if i < len(colWidths)-1 {
			top.WriteString(tblTM)
		} else {
			top.WriteString(tblTR)
		}
	}
	out = append(out, top.String())

	var headerLine strings.Builder
	headerLine.WriteString(tblV)
	for i, cell := range header {
		headerLine.WriteString(" " + padCell(cell, colWidths[i], "left") + " " + tblV)
	}
	out = append(out, headerLine.String())

	var sep strings.Builder
	sep.WriteString(tblML)
	for i, w := range colWidths {
		sep.WriteString(strings.Repeat(tblH, w+2))
		if i < len(colWidths)-1 {
			sep.WriteString(tblMM)
		} else {
			sep.WriteString(tblMR)
		}
	}
	out = append(out, sep.String())

	for _, row := range rows {
		var rowLine strings.Builder
		rowLine.WriteString(tblV)
		for i := 0; i < numCols; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			al := "left"
			if i < len(alignments) {
				al = alignments[i]
			}
			rowLine.WriteString(" " + padCell(cell, colWidths[i], al) + " " + tblV)
		}
		out = append(out, rowLine.String())
	}

	var bot strings.Builder
	bot.WriteString(tblBL)
	for i, w := range colWidths {
		bot.WriteString(strings.Repeat(tblH, w+2))
		if i < len(colWidths)-1 {
			bot.WriteString(tblBM)
		} else {
			bot.WriteString(tblBR)
		}
	}
	out = append(out, bot.String())

	if a := tableAlignCode(align); a != "" {
		out = append(out, "`a")
	}
	return out
}

func tableAlignCode(align string) string {
	switch strings.ToLower(strings.TrimSpace(align)) {
	case "l", "left":
		return "l"
	case "c", "center":
		return "c"
	case "r", "right":
		return "r"
	default:
		return ""
	}
}
