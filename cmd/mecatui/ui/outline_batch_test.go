package ui

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/stacklok/mecatl/cmd/mecatui/client"
	"github.com/stacklok/mecatl/cmd/mecatui/theme"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/scrollback"
)

func TestOutlineBatchActorColumns(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m.rend.setWidth(80)
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1})
	for i, id := range []string{"a", "b", "c", "d"} {
		m = applyAll(m, client.ToolCallMsg{ID: id, Name: "Read", Args: `{"path":"` + id + `.go"}`}, client.ToolResultMsg{CallID: id, Content: "ok"})
		frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
		var rows []string
		for j, p := range frame.provenance {
			if p.kind == scrollback.KindTool {
				rows = append(rows, ansi.Strip(frame.lines[j]))
			}
		}
		for j, row := range rows {
			want := "│ ├ ✓ Read"
			if j == len(rows)-1 {
				want = "│ ╰ ✓ Read"
			}
			if j == 0 {
				want = "├●┬ ✓ Read"
				if i == 0 {
					want = "├●─ ✓ Read"
				}
			}
			if !strings.HasPrefix(row, want) {
				t.Fatalf("growth %d row %d: got %q, want prefix %q", i, j, row, want)
			}
		}
		if strings.Count(strings.Join(rows, "\n"), "●") != 1 {
			t.Fatal("batch actor repeated")
		}
	}
}

func TestOutlineDetailedDelegationRails(t *testing.T) {
	for _, name := range []string{"Subagent", "Team"} {
		t.Run(name, func(t *testing.T) {
			m := outlineTestModel()
			m.phase = phaseRunning
			m.rend.setWidth(100)
			m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "call", Name: name, Args: `{}`})
			if name == "Subagent" {
				m = applyAll(m, client.SubagentMsg{Kind: client.SubagentStart, ParentCallID: "call", ChildID: "child", Goal: "Fix interrupted outline lifecycle placement", RoutedCategory: "category", RoutedModel: "model", Model: "model", Provider: "provider"})
			} else {
				m = applyAll(m, client.TeamMsg{Kind: client.TeamStart, ParentCallID: "call", TeamID: "team", Roster: []client.TeamMemberSpec{{Name: "lead", Lead: true}}})
			}
			id := uint64(m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID)
			check := func(pending bool) {
				t.Helper()
				frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
				var rows []string
				for i, p := range frame.provenance {
					if p.blockID == id {
						rows = append(rows, strings.TrimRight(ansi.Strip(frame.lines[i]), " "))
					}
				}
				if len(rows) < 2 || !strings.HasPrefix(rows[0], "├●╮ … ") {
					t.Fatalf("detailed opener: %q", rows)
				}
				for i := 1; i < len(rows); i++ {
					want := "│ │ "
					if !pending && i == len(rows)-1 {
						want = "│ ╰ "
					}
					if !strings.HasPrefix(rows[i], want) {
						t.Fatalf("pending=%t detail %d: %q, want %q", pending, i, rows[i], want)
					}
				}
				if pending && name == "Subagent" && !strings.Contains(strings.Join(rows, "\n"), "│ │ ↳ Fix interrupted outline lifecycle placement\n│ │ routed: category → provider/model\n│ │ subagent ·") {
					t.Fatalf("operator grammar: %q", rows)
				}
			}
			check(true)
			if name == "Subagent" {
				m = applyAll(m, client.SubagentMsg{Kind: client.SubagentEnd, ParentCallID: "call", ChildID: "child", Stop: "end_turn"})
			} else {
				m = applyAll(m, client.TeamMsg{Kind: client.TeamEnd, ParentCallID: "call", TeamID: "team", Stop: "end_turn"})
			}
			check(false) // Lifecycle done, canonical result not yet received: still detailed.
			m = applyAll(m, client.ToolResultMsg{CallID: "call", Content: "│ ✓ running text is not lifecycle"})
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			for i, p := range frame.provenance {
				if p.blockID == id && !strings.HasPrefix(ansi.Strip(frame.lines[i]), "├●─ ✓ "+name) {
					t.Fatalf("canonical settled singleton: %q", frame.lines[i])
				}
			}
		})
	}
}

func TestOutlineBatchMultilineGrowthAndResults(t *testing.T) {
	for _, name := range []string{"Subagent", "Team"} {
		for position := range 3 {
			t.Run(fmt.Sprintf("%s/position=%d", name, position), func(t *testing.T) {
				m := outlineTestModel()
				m.phase = phaseRunning
				m.rend.setWidth(28)
				m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1})
				var ids []uint64
				var anchor readingAnchor
				for i := range 3 {
					callID := fmt.Sprintf("call-%d", i)
					tool := "Shell"
					args := `{"command":"echo one two three four five six seven eight nine"}`
					if i == position {
						tool, args = name, `{}`
					}
					m = applyAll(m, client.ToolCallMsg{ID: callID, Name: tool, Args: args})
					if i == position {
						if name == "Subagent" {
							m = applyAll(m, client.SubagentMsg{Kind: client.SubagentStart, ParentCallID: callID, ChildID: "child", Goal: "wrapped goal with 界 emoji 🌍 and several words"})
						} else {
							m = applyAll(m, client.TeamMsg{Kind: client.TeamStart, ParentCallID: callID, TeamID: "team", Roster: []client.TeamMemberSpec{{Name: "a long member name", Lead: true}}})
						}
					}
					ids = append(ids, uint64(m.conv.scrollback.MetadataAt(m.conv.scrollback.Len()-1).ID))
					frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
					if i == 0 {
						var ok bool
						anchor, ok = frame.anchorForRow(frame.firstRegionRow(ids[0], conversationRegionArguments))
						if !ok || !anchor.text {
							t.Fatal("detail anchor missing")
						}
					} else if _, ok := frame.rowForAnchor(anchor); !ok {
						t.Fatal("growth lost detail anchor")
					}
					for j, id := range ids {
						var rows []string
						for k, p := range frame.provenance {
							if p.blockID == id {
								rows = append(rows, ansi.Strip(frame.lines[k]))
							}
							if ansi.StringWidth(frame.lines[k]) > 28 {
								t.Fatalf("overflow: %q", frame.lines[k])
							}
						}
						want := "│ ├ "
						if j == i {
							want = "│ ╰ "
						}
						if j == 0 {
							want = "├●┬ "
							if i == 0 {
								want = "├●╮ "
							}
						}
						if len(rows) < 2 || !strings.HasPrefix(rows[0], want) {
							t.Fatalf("growth=%d item=%d opener: %q want %q", i, j, rows, want)
						}
						for _, row := range rows[1:] {
							if !strings.HasPrefix(row, "│ │ ") {
								t.Fatalf("pending detail rail: %q", row)
							}
						}
					}
				}
				before := m.rend.renderConversationFrame(&m.conv.scrollback, true)
				loads, renders := m.rend.snapshotLoads, m.rend.blockRenders
				cached := m.rend.renderConversationFrame(&m.conv.scrollback, true)
				if strings.Join(cached.lines, "\n") != strings.Join(before.lines, "\n") || m.rend.snapshotLoads != loads || m.rend.blockRenders != renders {
					t.Fatal("frame reuse changed output or loaded unchanged snapshots")
				}
				// Result text cannot decide whether the final sibling's detail rail is live.
				callID := fmt.Sprintf("call-%d", position)
				m = applyAll(m, client.ToolResultMsg{CallID: callID, Content: "✓ done running │ text", Available: true})
				available := m.rend.renderConversationFrame(&m.conv.scrollback, true)
				for i, p := range available.provenance {
					if p.blockID == ids[position] && p.region == conversationRegionResult && !strings.HasPrefix(ansi.Strip(available.lines[i]), "│ │ ") {
						t.Fatalf("provisional result prematurely closed rail: %q", available.lines[i])
					}
				}
				for i := range 3 {
					if i == 2 && position == 2 {
						for _, terminal := range []bool{false, true} {
							if terminal {
								if name == "Subagent" {
									m = applyAll(m, client.SubagentMsg{Kind: client.SubagentEnd, ParentCallID: callID, ChildID: "child", Stop: "end_turn"})
								} else {
									m = applyAll(m, client.TeamMsg{Kind: client.TeamEnd, ParentCallID: callID, TeamID: "team", Stop: "end_turn"})
								}
							}
							metadata, ok := m.conv.scrollback.ToolCallMetadataAt(m.conv.scrollback.Len() - 1)
							if !ok || !metadata.Provisional || metadata.Terminal != terminal {
								t.Fatalf("retained detail lifecycle: %+v", metadata)
							}
							frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
							last, count := -1, 0
							for k, p := range frame.provenance {
								if p.blockID == ids[2] {
									last, count = k, count+1
								}
							}
							if count < 2 || frame.provenance[last].region != conversationRegionResult {
								t.Fatalf("terminal=%t: expected retained multiline result, rows=%q", terminal, frame.lines)
							}
							want := "│ │ ✓ done running │ text"
							if terminal {
								want = "│ ╰ ✓ done running │ text"
							}
							if got := strings.TrimRight(ansi.Strip(frame.lines[last]), " "); got != want {
								t.Fatalf("terminal=%t final retained detail: got %q, want %q", terminal, got, want)
							}
						}
					}
					m = applyAll(m, client.ToolResultMsg{CallID: fmt.Sprintf("call-%d", i), Content: "running │ …", IsError: i == 1})
					frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
					if strings.Count(ansi.Strip(strings.Join(frame.lines, "\n")), "●") != 1 {
						t.Fatal("result arrival repeated actor")
					}
					for k, p := range frame.provenance {
						if p.blockID == ids[2] && p.region == conversationRegionArguments && i < 2 && !strings.HasPrefix(ansi.Strip(frame.lines[k]), "│ │ ") {
							t.Fatalf("earlier result closed pending final detail: %q", frame.lines[k])
						}
					}
				}
			})
		}
	}
}

func TestOutlineWrappedHeaderWithoutDetail(t *testing.T) {
	m := outlineTestModel()
	m.phase = phaseRunning
	m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "long", Name: strings.Repeat("LongTool", 8) + "\x1b[2J", Args: ""})
	id := uint64(m.conv.scrollback.MetadataAt(m.conv.scrollback.Len() - 1).ID)
	for _, width := range []int{100, 40, 16, 8, 5, 2, 1} {
		m.rend.setWidth(width)
		frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
		count := 0
		for i, p := range frame.provenance {
			if ansi.StringWidth(frame.lines[i]) > width || strings.Contains(frame.lines[i], "\x1b[2J") {
				t.Fatalf("width=%d unsafe/overflow: %q", width, frame.lines[i])
			}
			if p.blockID != id {
				continue
			}
			plain := ansi.Strip(frame.lines[i])
			if width > 5 {
				want := "│ │ "
				if count == 0 {
					want = "├●─ " // Wrapping alone does not invent a detailed opener.
				}
				if !strings.HasPrefix(plain, want) || p.rail != 4 || p.region != conversationRegionChrome {
					t.Fatalf("width=%d header row=%d: %q %+v", width, count, plain, p)
				}
			}
			count++
		}
		if width == 100 && count != 1 {
			t.Fatalf("single-line pending call gained phantom details: %d rows", count)
		}
		if width > 5 && width < 100 && count < 2 {
			t.Fatalf("width=%d header did not wrap", width)
		}
	}
}

func TestOutlineDelegationSelectionAndMonochrome(t *testing.T) {
	for _, name := range []string{"Subagent", "Team"} {
		t.Run(name, func(t *testing.T) {
			m := outlineTestModel()
			m.phase = phaseRunning
			m.rend.setWidth(80)
			m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "call", Name: name, Args: `{}`})
			if name == "Subagent" {
				m = applyAll(m, client.SubagentMsg{Kind: client.SubagentStart, ParentCallID: "call", ChildID: "child", Goal: "ANCHOR followed by enough words to wrap 界 🌍"})
			} else {
				m = applyAll(m, client.TeamMsg{Kind: client.TeamStart, ParentCallID: "call", TeamID: "team", Roster: []client.TeamMemberSpec{{Name: "ANCHOR", Lead: true}}})
			}
			frame := m.rend.renderConversationFrame(&m.conv.scrollback, true)
			var sel selection
			var anchor readingAnchor
			for i, row := range frame.provenance {
				if row.region != conversationRegionArguments {
					continue
				}
				text, leading := selectionRowText(frame, row, frame.lines[i])
				if at := strings.Index(text, "ANCHOR"); at >= 0 {
					col := leading + graphemeCount(text[:at])
					sel = selection{active: true, anchorL: i, headL: i, anchorC: col, headC: col + 6}
					anchor, _ = frame.anchorForRow(i)
					break
				}
			}
			if !sel.snapshotLogical(frame, strings.Join(frame.lines, "\n")) || sel.copied != "ANCHOR" {
				t.Fatalf("initial delegation selection: %+v", sel)
			}
			offset := sel.anchorPoint.sourceOffset
			m = applyAll(m, client.ToolCallMsg{ID: "sibling", Name: "Read", Args: `{"path":"sibling.go"}`})
			for _, expand := range []bool{true, false, true} {
				for _, width := range []int{48, 80} {
					m.rend.setWidth(width)
					frame = m.rend.renderConversationFrame(&m.conv.scrollback, expand)
					if !sel.resolveLogical(frame) || sel.copied != "ANCHOR" || sel.anchorPoint.sourceOffset != offset {
						t.Fatalf("width=%d expanded=%t selection: %+v", width, expand, sel)
					}
					if _, ok := frame.rowForAnchor(anchor); !ok {
						t.Fatalf("width=%d expanded=%t anchor lost", width, expand)
					}
					mono := newRenderer(theme.New("monochrome", theme.Palette{}), m.rend.marks)
					mono.setWidth(width)
					monochrome := mono.renderConversationFrame(&m.conv.scrollback, expand)
					if ansi.Strip(strings.Join(frame.lines, "\n")) != ansi.Strip(strings.Join(monochrome.lines, "\n")) {
						t.Fatalf("width=%d expanded=%t monochrome geometry changed", width, expand)
					}
				}
			}
			m = applyAll(m, client.ToolResultMsg{CallID: "sibling", Content: "ok"})
			if !sel.resolveLogical(m.rend.renderConversationFrame(&m.conv.scrollback, true)) {
				t.Fatal("sibling result lost delegation selection")
			}
		})
	}
}

func TestOutlineInnerRailStyles(t *testing.T) {
	m := outlineTestModel()
	r := m.rend
	for _, glyph := range []string{"╭◆╮", "├◆─", "├●┬", "├●─", "├●╮", "│ ├", "│ ╰", "│ │"} {
		chars := []rune(glyph)
		want := r.th.Style("runRail").Render(string(chars[0])) + " "
		switch chars[1] {
		case '◆':
			want = r.th.Style("runRail").Render(string(chars[0])) + r.th.Style("assistantLabel").Render("◆")
		case '●':
			want = r.th.Style("runRail").Render(string(chars[0])) + r.th.Style("toolName").Render("●")
		}
		want += r.th.Style("turnRail").Render(string(chars[2]))
		if got := r.outlineGlyph(glyph); got != want {
			t.Errorf("%s: got %q, want %q", glyph, got, want)
		}
	}
	if r.th.Style("turnRail").Render("│") == r.th.Style("runRail").Render("│") || r.th.Style("turnRail").Render("●") == r.th.Style("toolName").Render("●") {
		t.Fatal("inner, outer, and actor style roles must remain independent")
	}
	t.Run("reducer-frame", func(t *testing.T) {
		m.phase = phaseRunning
		r.setWidth(80)
		m = applyAll(m, client.SessionInitMsg{}, client.TurnStartMsg{Turn: 1}, client.AssistantDeltaMsg{Turn: 1, Text: "model first\n\nmodel continuation"}, client.TurnEndMsg{Turn: 1}, client.ToolCallMsg{ID: "shell", Name: "Shell", Args: `{"command":"echo tool detail"}`})
		frame := r.renderConversationFrame(&m.conv.scrollback, true)
		modelActor, toolActor, modelRail, toolRail := 0, 0, 0, 0
		for i, row := range frame.provenance {
			line := frame.lines[i]
			plain := ansi.Strip(line)
			if strings.HasPrefix(plain, "╭◆╮") || strings.HasPrefix(plain, "├●╮") {
				actorStyle, actor, outer := "assistantLabel", "◆", "╭"
				if row.kind == scrollback.KindTool {
					actorStyle, actor, outer = "toolName", "●", "├"
					toolActor++
				} else {
					modelActor++
				}
				// Cut retains offscreen SGR sequences; require the exact styled visible glyph.
				if got, want := ansi.Cut(line, 0, 1), r.th.Style("runRail").Render(outer); !strings.Contains(got, want) {
					t.Errorf("%s outer rail: got %q, want %q", actor, got, want)
				}
				if got, want := ansi.Cut(line, 1, 2), r.th.Style(actorStyle).Render(actor); !strings.Contains(got, want) {
					t.Errorf("%s actor: got %q, want %q", actor, got, want)
				}
				if got, want := ansi.Cut(line, 2, 3), r.th.Style("turnRail").Render("╮"); !strings.Contains(got, want) {
					t.Errorf("%s inner opener: got %q, want %q", actor, got, want)
				}
			}
			if !strings.HasPrefix(plain, "│ │ ") {
				continue
			}
			if row.kind == scrollback.KindAssistant && row.region == conversationRegionBody {
				modelRail++
			} else if row.kind == scrollback.KindTool && row.region == conversationRegionArguments {
				toolRail++
			} else {
				continue
			}
			if got, want := ansi.Cut(line, 0, 1), r.th.Style("runRail").Render("│"); !strings.Contains(got, want) {
				t.Errorf("%v outer continuation: got %q, want %q", row.kind, got, want)
			}
			if got, want := ansi.Cut(line, 2, 3), r.th.Style("turnRail").Render("│"); !strings.Contains(got, want) {
				t.Errorf("%v inner continuation: got %q, want %q", row.kind, got, want)
			}
		}
		if modelActor != 1 || toolActor != 1 || modelRail == 0 || toolRail == 0 {
			t.Fatalf("missing production style samples: model actor=%d tool actor=%d model rail=%d tool rail=%d", modelActor, toolActor, modelRail, toolRail)
		}
	})
}
