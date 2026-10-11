package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/stacklok/mecatl/cmd/mecatui/internal/renderfmt"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/scrollback"
)

// outlineFrameBuilder keeps the observed membership and per-frame rail state
// together while projecting cached card rows into lockstep frame rows.
type outlineFrameBuilder struct {
	r              *renderer
	source, output renderedFrame
	runs           []scrollback.RunOutline
	byCard         map[uint64]scrollback.CardOutline
	byRun          map[scrollback.RunID]scrollback.RunOutline
	endByRun       map[scrollback.RunID]uint64
	positions      map[uint64]int
	pending        int
	first          map[uint64]bool
	started        map[scrollback.RunID]bool
	expand         bool
	toolGlyph      map[uint64]string
	toolLast       map[uint64]bool
	toolLive       map[uint64]bool
	toolDetail     map[uint64]bool
	toolRows       map[uint64]int
}

// outlinedFrame adds presentation-only chrome to cached, width-budgeted card rows.
// Card provenance is carried forward; only the new rows are chrome.
func (r *renderer) outlinedFrame(c *scrollback.Conversation, frame renderedFrame, expand bool) renderedFrame {
	b := newOutlineFrameBuilder(r, c, frame)
	b.expand = expand
	for i, line := range frame.lines {
		row := frame.provenance[i]
		if expand {
			if row.blockID != 0 {
				b.appendPending(b.positions[row.blockID])
			} else if i == len(frame.lines)-1 {
				b.appendPending(c.Len())
			}
		}
		if role, ok := b.byCard[row.blockID]; ok {
			if b.appendOutlined(line, row, role) {
				continue // obsolete expanded speaker label
			}
		} else if expand && row.separator && b.inRunSeparator(i) {
			continue // presentation-only gaps between observed structural cards
		} else {
			// Unobserved snapshot cards (including the prompt) keep their flat bytes.
			b.output.lines = append(b.output.lines, line)
			b.output.provenance = append(b.output.provenance, row)
		}
		if expand && row.blockID != 0 && (i+1 == len(frame.lines) || frame.provenance[i+1].blockID != row.blockID) {
			b.appendPending(b.positions[row.blockID] + 1)
		}
	}
	return b.output
}

func newOutlineFrameBuilder(r *renderer, c *scrollback.Conversation, frame renderedFrame) *outlineFrameBuilder {
	runs := c.Outline().Runs()
	b := &outlineFrameBuilder{
		r: r, source: frame, runs: runs,
		output: renderedFrame{lines: make([]string, 0, len(frame.lines)+len(runs)*2), provenance: make([]renderedRow, 0, len(frame.provenance)+len(runs)*2), appendixID: frame.appendixID},
		byCard: make(map[uint64]scrollback.CardOutline), byRun: make(map[scrollback.RunID]scrollback.RunOutline, len(runs)),
		endByRun: make(map[scrollback.RunID]uint64, len(runs)), positions: map[uint64]int{0: -1},
		first: make(map[uint64]bool), started: make(map[scrollback.RunID]bool),
		toolGlyph: make(map[uint64]string), toolLast: make(map[uint64]bool),
		toolLive: make(map[uint64]bool), toolDetail: make(map[uint64]bool), toolRows: make(map[uint64]int),
	}
	for _, run := range runs {
		b.byRun[run.ID] = run
		b.endByRun[run.ID] = uint64(run.StartAfter)
		batches := make(map[uint64][]uint64)
		for _, tool := range run.Tools {
			batches[tool.BatchID] = append(batches[tool.BatchID], uint64(tool.CardID))
		}
		for _, members := range batches {
			for i, id := range members {
				glyph := "│ ├"
				if i == len(members)-1 {
					glyph = "│ ╰"
					b.toolLast[id] = true
				}
				if i == 0 {
					glyph = "├●┬"
					if len(members) == 1 {
						glyph = "├●─"
					}
				}
				b.toolGlyph[id] = glyph
			}
		}
	}
	for i := 0; i < c.Len(); i++ {
		id := c.MetadataAt(i).ID
		b.positions[uint64(id)] = i
		if role, ok := c.Outline().Card(id); ok {
			b.byCard[uint64(id)] = role
			b.endByRun[role.RunID] = uint64(id)
			if role.Tool {
				metadata, _ := c.ToolCallMetadataAt(i)
				b.toolLive[uint64(id)] = !metadata.Terminal && (!metadata.ResultReceived || metadata.Provisional)
			}
		}
	}
	for _, row := range frame.provenance {
		if _, ok := b.toolGlyph[row.blockID]; ok {
			b.toolRows[row.blockID]++
			if row.region == conversationRegionArguments || row.region == conversationRegionResult {
				b.toolDetail[row.blockID] = true
			}
		}
	}
	return b
}

func (b *outlineFrameBuilder) add(line string, row renderedRow) {
	if b.r.width > 0 && b.r.width <= 5 {
		if row.rail == 4 {
			prefixWidth := max(0, b.r.width-2)
			line = ansi.Cut(line, 0, prefixWidth) + ansi.Cut(line, 4, ansi.StringWidth(line))
			row.indent, row.rail = prefixWidth, prefixWidth
			row.leading = max(0, row.leading-4+prefixWidth)
		}
		line = ansi.TruncateWc(line, b.r.width, "") // degenerate terminals cannot fit the full grammar.
	}
	b.output.lines = append(b.output.lines, line)
	b.output.provenance = append(b.output.provenance, row)
}

func (b *outlineFrameBuilder) chrome(glyph, content string, row renderedRow) {
	prefix := b.r.outlineGlyph(glyph)
	if content == "" {
		b.add(prefix, row)
		return
	}
	prefix += " "
	width := b.r.width - ansi.StringWidth(prefix)
	if width <= 0 {
		b.add(prefix, row)
		return
	}
	for i, segment := range strings.Split(ansi.Hardwrap(ansi.Wrap(content, width, " · "), width, true), "\n") {
		if i > 0 {
			prefix = b.r.outlineGlyph("│") + strings.Repeat(" ", ansi.StringWidth(prefix)-1)
		}
		b.add(prefix+segment, row)
	}
}

// appendPending projects each run at its recorded card boundary, including cards
// with no rendered rows. Runs sharing a boundary retain their observed order.
func (b *outlineFrameBuilder) appendPending(before int) {
	for b.pending < len(b.runs) {
		run := b.runs[b.pending]
		if b.positions[b.endByRun[run.ID]] >= before {
			break
		}
		b.pending++
		if len(run.Turns) > 0 {
			turn := run.Turns[len(run.Turns)-1]
			if !turn.Ended && turn.Visible && !b.first[uint64(turn.CardID)] {
				glyph := "├◆╮"
				if !b.started[run.ID] {
					glyph = "╭◆╮"
				}
				b.add(b.r.outlineGlyph(glyph), renderedRow{blockID: uint64(turn.CardID), region: conversationRegionChrome, kind: scrollback.KindAssistant})
				b.started[run.ID] = true
			}
		}
		if run.Status != scrollback.RunOpen && (len(run.Turns) > 0 || run.Status == scrollback.RunInterrupted) {
			b.appendRunFooter(run, renderedRow{blockID: b.endByRun[run.ID], region: conversationRegionChrome})
		}
	}
}

// appendOutlined returns true only for the discarded old assistant label rows.
func (b *outlineFrameBuilder) appendOutlined(line string, row renderedRow, role scrollback.CardOutline) bool {
	run := b.byRun[role.RunID]
	switch {
	case row.kind == scrollback.KindUser:
		glyph := "   "
		if !role.InitialPrompt && b.expand {
			glyph = "│  "
		}
		if !b.first[row.blockID] {
			b.first[row.blockID] = true
			glyph = " ▌ "
			if !role.InitialPrompt && b.expand {
				glyph = "├▌ "
			}
		}
		row.indent, row.rail, row.leading = 3, 3, 3
		prefix := b.r.outlineGlyph(glyph)
		if b.expand {
			prefix = b.r.outlineGlyph(strings.TrimSuffix(glyph, " ")) + b.r.th.Style("userLabel").Render("│") + " "
			row.indent, row.rail, row.leading = 4, 4, 4
		}
		b.add(prefix+line, row)
	case !b.expand && row.kind != scrollback.KindAssistant:
		b.add(line, row)
	case role.TurnID != 0 && row.kind == scrollback.KindTurnStat:
		b.appendTurnStat(line, row, run, role.TurnID)
	case role.TurnID != 0 && row.kind == scrollback.KindAssistant:
		return b.appendAssistant(line, row, run)
	default:
		b.appendActivity(line, row, run)
	}
	return false
}

func (b *outlineFrameBuilder) appendTurnStat(line string, row renderedRow, run scrollback.RunOutline, turnID scrollback.TurnID) {
	turn := outlineTurn(run, turnID)
	glyph := "│ ╰─"
	if !turn.Visible {
		glyph = "├◆─"
		if !b.started[run.ID] {
			glyph = "╭◆─"
		}
	}
	if b.first[row.blockID] {
		glyph = "│   "
		if !turn.Visible {
			glyph = "│  "
		}
	}
	b.first[row.blockID] = true
	content := strings.TrimSpace(line)
	b.started[run.ID] = true
	b.chrome(glyph, content, renderedRow{blockID: row.blockID, region: conversationRegionChrome, kind: row.kind})
}

func (b *outlineFrameBuilder) appendAssistant(line string, row renderedRow, run scrollback.RunOutline) bool {
	// The old expanded speaker label and its blank row are replaced by the opener.
	if b.expand && row.row < 2 && row.region == conversationRegionChrome {
		return true
	}
	glyph := "│ │"
	if !b.expand {
		glyph = "   "
	}
	if !b.first[row.blockID] {
		b.first[row.blockID] = true
		glyph = "├◆╮"
		if !b.started[run.ID] {
			glyph = "╭◆╮"
		}
		if !b.expand {
			glyph = " ◆ "
		}
		b.started[run.ID] = true
	}
	content := strings.TrimPrefix(line, strings.Repeat(" ", b.r.indent))
	content = strings.TrimPrefix(content, "  ") // obsolete speaker-label hanging indent
	if content != "" {
		line = b.r.outlineGlyph(glyph) + " " + content
	} else {
		line = b.r.outlineGlyph(glyph)
	}
	row.indent, row.rail = 4, 4
	row.leading = max(0, row.leading+row.indent-b.r.indent-2)
	b.add(line, row)
	return false
}

func (b *outlineFrameBuilder) appendActivity(line string, row renderedRow, run scrollback.RunOutline) {
	if !b.started[run.ID] {
		b.output.lines = append(b.output.lines, line)
		b.output.provenance = append(b.output.provenance, row)
		return
	}
	content := strings.TrimPrefix(line, strings.Repeat(" ", b.r.indent))
	glyph := "│"
	prefixWidth := 2
	if toolGlyph, ok := b.toolGlyph[row.blockID]; ok {
		b.toolRows[row.blockID]--
		glyph, prefixWidth = "│ │", 4
		if b.toolLast[row.blockID] && !b.toolLive[row.blockID] && b.toolRows[row.blockID] == 0 {
			glyph = "│ ╰"
		}
		if !b.first[row.blockID] {
			glyph = toolGlyph
			if glyph == "├●─" && b.toolDetail[row.blockID] {
				glyph = "├●╮"
			}
			b.first[row.blockID] = true
		}
	}
	line = b.r.outlineGlyph(glyph) + " " + content
	row.indent, row.rail = prefixWidth, prefixWidth
	row.leading = max(0, row.leading+prefixWidth-b.r.indent)
	b.add(line, row)
}

func (b *outlineFrameBuilder) inRunSeparator(index int) bool {
	previous, next := index-1, index+1
	for previous >= 0 && b.source.provenance[previous].separator {
		previous--
	}
	for next < len(b.source.provenance) && b.source.provenance[next].separator {
		next++
	}
	if previous >= 0 && next < len(b.source.provenance) {
		left, leftOK := b.byCard[b.source.provenance[previous].blockID]
		right, rightOK := b.byCard[b.source.provenance[next].blockID]
		return leftOK && rightOK && left.RunID == right.RunID && b.started[left.RunID]
	}
	return false
}

func (b *outlineFrameBuilder) appendRunFooter(run scrollback.RunOutline, row renderedRow) {
	if len(run.Turns) > 0 {
		turn := run.Turns[len(run.Turns)-1]
		if turn.Visible && !turn.Ended {
			b.chrome("│ ╰─", "", renderedRow{blockID: uint64(turn.CardID), region: conversationRegionChrome})
		}
		if !turn.Visible && !turn.Ended {
			glyph := "├◆─"
			if !b.started[run.ID] {
				glyph = "╭◆─"
			}
			b.chrome(glyph, "no visible output", renderedRow{blockID: uint64(turn.CardID), region: conversationRegionChrome})
			b.started[run.ID] = true
		}
	}
	if !b.started[run.ID] {
		b.add(b.r.outlineGlyph("╭"), row)
		b.started[run.ID] = true
	}
	b.chrome("╰─", outlineFooter(run), row)
}

func outlineTurn(run scrollback.RunOutline, id scrollback.TurnID) scrollback.TurnOutline {
	for _, turn := range run.Turns {
		if turn.ID == id {
			return turn
		}
	}
	return scrollback.TurnOutline{}
}

func (r *renderer) outlineGlyph(glyph string) string {
	var b strings.Builder
	for index, ch := range glyph {
		if ch == ' ' {
			b.WriteByte(' ')
			continue
		}
		style := "runRail"
		switch ch {
		case '▌':
			style = "userLabel"
		case '◆':
			style = "assistantLabel"
		case '●':
			style = "toolName"
		default:
			if index > 0 && (strings.ContainsAny(glyph, "◆●") || strings.HasPrefix(glyph, "│ ")) {
				style = "turnRail"
			}
		}
		b.WriteString(r.th.Style(style).Render(string(ch)))
	}
	return b.String()
}

func outlineFooter(run scrollback.RunOutline) string {
	parts := make([]string, 0, 5)
	if run.UsageAvailable {
		parts = append(parts, "↑"+renderfmt.HumanizeTokens(run.Usage.InputTokens)+" ↓"+renderfmt.HumanizeTokens(run.Usage.OutputTokens))
	}
	if run.LLMDurationMs > 0 {
		parts = append(parts, "LLM "+renderfmt.FormatDuration(run.LLMDurationMs))
	}
	if run.ToolDurationMs > 0 {
		parts = append(parts, "tools "+renderfmt.FormatDuration(run.ToolDurationMs))
	}
	if run.UsageAvailable && run.Usage.InputTokens > 0 && run.Usage.CacheReadTokens > 0 {
		parts = append(parts, renderfmt.PctString(float64(run.Usage.CacheReadTokens)/float64(run.Usage.InputTokens))+" cached")
	}
	switch run.Status {
	case scrollback.RunNormal:
	case scrollback.RunError:
		parts = append(parts, "error")
	case scrollback.RunInterrupted:
		parts = append(parts, "interrupted")
	}
	return strings.Join(parts, " · ")
}
