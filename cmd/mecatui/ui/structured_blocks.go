package ui

import (
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/stacklok/mecatl/cmd/mecatui/internal/terminaltext"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/blocks"
	"github.com/stacklok/mecatl/cmd/mecatui/ui/internal/scrollback"
)

func (r *renderer) plainBlockLayout(expand bool) blocks.PlainLayout {
	return blocks.SnapshotPlainLayout(blocks.PlainLayoutInput{
		Width: r.contentWidth(), Expanded: expand, ExpandMark: r.marks.expandConversation,
	})
}

// blockTheme maps UI stylesheet roles to the concrete semantic styles used by
// structured block preparation.
func (r *renderer) blockTheme() blocks.Theme {
	toolCard, _, _ := r.toolCardLayout()
	return blocks.Theme{
		UserLabel:    r.th.Style("userLabel"),
		UserBody:     r.th.Style("userBlock"),
		Muted:        r.th.Style("muted"),
		Warning:      r.th.Style("warning"),
		Error:        r.th.Style("errorText"),
		HookModified: r.th.Style("hookModified"),
		HookAdvisory: r.th.Style("hookAdvisory"),
		DeliveryHead: r.th.Style("hookModified"),
		ToolCard:     toolCard,
	}
}

func (r *renderer) renderPreparedSnapshot(index int, id scrollback.BlockID, revision uint64, kind scrollback.Kind, expand bool, prepare func() blocks.Prepared) string {
	return r.renderCachedSnapshot(index, uint64(id), rendererRevision(revision), expand, func(blockID uint64) blockRenderOutput {
		r.cardPrepares++
		prepared := prepare()
		return blockRenderOutput{
			text: r.indentLines(prepared.Text()),
			rows: blockProvenanceRows(prepared, blockID, kind, r.indent, r.width),
		}
	})
}

func (r *renderer) renderUserSnapshot(index int, s scrollback.BlockSnapshot, p scrollback.UserCardSnapshot, expand bool) string {
	if r.outlineInset > 0 {
		return r.renderCachedSnapshot(index, uint64(s.ID), rendererRevision(s.Revision), expand, func(blockID uint64) blockRenderOutput {
			content := terminaltext.Sanitize(p.Text)
			for _, media := range p.Media {
				if content != "" {
					content += "\n"
				}
				content += r.th.Style("muted").Render("📎 " + terminaltext.Sanitize(media))
			}
			content = normalizeEmojiWidth(content)
			source := ansi.Strip(content)
			out := ansi.Hardwrap(ansi.Wrap(content, max(1, r.width-r.outlineInset), ""), max(1, r.width-r.outlineInset), true)
			rows := make([]renderedRow, strings.Count(out, "\n")+1)
			cursor, offset := 0, 0
			for i, line := range strings.Split(out, "\n") {
				rows[i] = renderedRow{blockID: blockID, region: conversationRegionChrome, kind: scrollback.KindUser, row: i}
				plain := strings.TrimRight(ansi.Strip(line), " ")
				at := strings.Index(source[cursor:], plain)
				if at < 0 {
					continue // Unmatched layout cannot safely supply selectable source coordinates.
				}
				// Soft-wrap whitespace belongs to the source; physical newlines are layout.
				offset += graphemeCount(strings.ReplaceAll(source[cursor:cursor+at], "\n", ""))
				rows[i].region, rows[i].text, rows[i].sourceOffset, rows[i].span = conversationRegionBody, plain != "", offset, graphemeCount(plain)
				offset += rows[i].span
				cursor += at + len(plain)
			}
			return blockRenderOutput{text: out, rows: rows}
		})
	}
	return r.renderPreparedSnapshot(index, s.ID, s.Revision, scrollback.KindUser, expand, func() blocks.Prepared { return r.prepareUserSnapshot(p) })
}

func (r *renderer) renderNoticeSnapshot(index int, s scrollback.BlockSnapshot, p scrollback.NoticeCardSnapshot, expand bool) string {
	if p.BenignGuardrail && !expand && !r.showBenignGuardrails {
		return r.renderHiddenSnapshot(index, s, expand)
	}
	return r.renderPreparedSnapshot(index, s.ID, s.Revision, scrollback.KindNotice, expand, func() blocks.Prepared { return r.prepareNoticeSnapshot(p) })
}

func (r *renderer) renderHookSnapshot(index int, s scrollback.BlockSnapshot, p scrollback.HookCardSnapshot, expand bool) string {
	return r.renderPreparedSnapshot(index, s.ID, s.Revision, scrollback.KindHook, expand, func() blocks.Prepared { return r.prepareHookSnapshot(p) })
}

func (r *renderer) renderHiddenSnapshot(index int, s scrollback.BlockSnapshot, expand bool) string {
	return r.renderCachedSnapshot(index, uint64(s.ID), rendererRevision(s.Revision), expand, func(uint64) blockRenderOutput {
		return blockRenderOutput{}
	})
}

func (r *renderer) renderTurnStatSnapshot(index int, s scrollback.BlockSnapshot, p scrollback.TurnStatCardSnapshot, expand bool) string {
	if !expand {
		return r.renderCachedSnapshot(index, uint64(s.ID), rendererRevision(s.Revision), false, func(uint64) blockRenderOutput {
			return blockRenderOutput{}
		})
	}
	return r.renderPreparedSnapshot(index, s.ID, s.Revision, scrollback.KindTurnStat, expand, func() blocks.Prepared { return r.prepareTurnStatSnapshot(p) })
}

func (r *renderer) renderErrorSnapshot(index int, s scrollback.BlockSnapshot, p scrollback.ErrorCardSnapshot, expand bool) string {
	return r.renderPreparedSnapshot(index, s.ID, s.Revision, scrollback.KindError, expand, func() blocks.Prepared { return r.prepareErrorSnapshot(p, expand) })
}

func (r *renderer) renderDeliverySnapshot(index int, s scrollback.BlockSnapshot, p scrollback.DeliveryCardSnapshot, expand bool) string {
	return r.renderPreparedSnapshot(index, s.ID, s.Revision, scrollback.KindDelivery, expand, func() blocks.Prepared { return r.prepareDeliverySnapshot(p) })
}

func (r *renderer) prepareUserSnapshot(p scrollback.UserCardSnapshot) blocks.Prepared {
	return blocks.PrepareUser(blocks.SnapshotUser(blocks.UserInput{Text: p.Text, Media: p.Media}), r.plainBlockLayout(false), r.blockTheme())
}

func (r *renderer) prepareNoticeSnapshot(p scrollback.NoticeCardSnapshot) blocks.Prepared {
	return blocks.PrepareNotice(blocks.SnapshotNotice(blocks.NoticeInput{Text: p.Text, Recovery: p.Recover}), r.plainBlockLayout(false), r.blockTheme())
}

func (r *renderer) prepareHookSnapshot(p scrollback.HookCardSnapshot) blocks.Prepared {
	return blocks.PrepareHook(blocks.SnapshotHook(blocks.HookInput{Text: p.Text, Phase: p.Phase, Tool: p.Tool, Decision: p.Decision}), r.plainBlockLayout(false), r.blockTheme())
}

func (r *renderer) prepareTurnStatSnapshot(p scrollback.TurnStatCardSnapshot) blocks.Prepared {
	return blocks.PrepareTurnStat(blocks.SnapshotTurnStat(blocks.TurnStatInput{Text: p.Text}), r.plainBlockLayout(false), r.blockTheme())
}

func (r *renderer) prepareErrorSnapshot(p scrollback.ErrorCardSnapshot, expand bool) blocks.Prepared {
	if p.Permanent {
		return blocks.PreparePermanentError(blocks.SnapshotPermanentError(blocks.PermanentErrorInput{Text: p.Text}), r.plainBlockLayout(expand), r.blockTheme())
	}
	return blocks.PrepareError(blocks.SnapshotError(blocks.ErrorInput{Text: p.Text}), r.plainBlockLayout(false), r.blockTheme())
}

func (r *renderer) prepareDeliverySnapshot(p scrollback.DeliveryCardSnapshot) blocks.Prepared {
	return blocks.PrepareDelivery(blocks.SnapshotDelivery(blocks.DeliveryInput{ScheduleName: p.ScheduleName, FireID: p.FireID, Text: p.Text}), r.plainBlockLayout(false), r.blockTheme())
}
