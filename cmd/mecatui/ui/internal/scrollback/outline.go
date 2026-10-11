package scrollback

// RunID identifies one observed live run within a Conversation. It is transient
// and is not a server or persisted identifier.
type RunID uint64

// TurnID identifies one observed model turn within a RunID.
type TurnID uint64

// RunStatus describes whether an observed run remains open or how it ended.
type RunStatus uint8

const (
	// RunOpen means no terminal event has been observed.
	RunOpen RunStatus = iota
	// RunNormal means the run received a normal terminal result.
	RunNormal
	// RunError means the run received an error terminal result.
	RunError
	// RunInterrupted means the live run was cancelled or abandoned.
	RunInterrupted
)

// TurnStats records one observed turn-end. A zero DurationMs is preserved; it
// means the source did not supply a positive model-call duration.
type TurnStats struct {
	Usage
	DurationMs int64
	Estimated  bool
}

// TurnOutline is a detached view of an observed turn. Visible is set by the
// reducer when assistant text or reasoning is observed; it is not inferred from
// card payload content.
type TurnOutline struct {
	ID      TurnID
	CardID  BlockID
	Visible bool
	Ended   bool
	Stats   TurnStats
}

// ToolOutline is a detached view of a tool card observed in a run. DurationMs
// is a stored prototype value, not a measured execution duration. BatchID is
// run-local synthetic membership; it does not imply concurrent execution.
type ToolOutline struct {
	CardID     BlockID
	DurationMs int64
	BatchID    uint64
}

// RunOutline is a detached view of one observed run. ToolDurationMs sums the
// maximum stored tool duration in each synthetic batch; LLMDurationMs sums only
// positive completed-turn durations.
type RunOutline struct {
	ID                            RunID
	StartAfter                    BlockID
	Status                        RunStatus
	Usage                         Usage
	UsageAvailable                bool
	Reconnecting                  bool
	LLMDurationMs, ToolDurationMs int64
	Turns                         []TurnOutline
	Tools                         []ToolOutline
}

// CardOutline identifies an observed structural role for a card. Cards without
// outline metadata remain absent from this projection.
type CardOutline struct {
	RunID         RunID
	TurnID        TurnID
	Tool          bool
	DurationMs    int64
	InitialPrompt bool
}

type runOutline struct {
	id                      RunID
	startAfter              BlockID
	status                  RunStatus
	usage                   Usage
	usageAvailable          bool
	reconnecting            bool
	turns                   []turnOutline
	tools                   []toolOutline
	llmDurationMs           int64
	completedToolMs         int64
	currentBatchMaxMs       int64
	batchEligible           bool
	nextBatch, currentBatch uint64
}

type turnOutline struct {
	id      TurnID
	cardID  BlockID
	visible bool
	ended   bool
	stats   TurnStats
}

type toolOutline struct {
	cardID     BlockID
	durationMs int64
	batchID    uint64
}

type outlineState struct {
	version           uint64
	nextRun, nextTurn uint64
	runs              []runOutline
	cards             map[BlockID]CardOutline
}

// OutlineCards records transient observed run and turn structure alongside a
// Conversation. It never changes card payloads, identities, or revisions.
type OutlineCards struct{ conversation *Conversation }

// Outline returns the facade for recording and querying transient run outlines.
func (c *Conversation) Outline() OutlineCards { return OutlineCards{conversation: c} }

// Version changes when observed outline presentation facts change.
func (o OutlineCards) Version() uint64 {
	if o.conversation.outline == nil {
		return 0
	}
	return o.conversation.outline.version
}

// StartRun records an observed run start and, when supplied, its live initial
// prompt. It returns zero while another observed run remains open.
func (o OutlineCards) StartRun(initialPrompt BlockID) RunID {
	state := o.state()
	if len(state.runs) > 0 && state.runs[len(state.runs)-1].status == RunOpen {
		return 0
	}
	state.nextRun++
	var after BlockID
	if n := o.conversation.Len(); n > 0 {
		after = o.conversation.MetadataAt(n - 1).ID
	}
	state.runs = append(state.runs, runOutline{id: RunID(state.nextRun), startAfter: after, status: RunOpen})
	state.version++
	if initialPrompt != 0 && initialPrompt == after && o.conversation.MetadataAt(o.conversation.Len()-1).Kind == KindUser {
		if _, assigned := o.Card(after); !assigned {
			o.card(after, CardOutline{RunID: RunID(state.nextRun), InitialPrompt: true})
		}
	}
	return RunID(state.nextRun)
}

// StartTurn records an observed turn start in an open run. cardID may be zero
// for a turn that has no associated card yet. A non-zero cardID must belong to
// this conversation and is associated without changing that card's payload.
func (o OutlineCards) StartTurn(runID RunID, cardID BlockID) TurnID {
	run := o.openRun(runID)
	if run == nil || activeTurn(run) != nil || (cardID != 0 && o.conversation.index(cardID) < 0) {
		return 0
	}
	state := o.state()
	state.nextTurn++
	turnID := TurnID(state.nextTurn)
	closeBatch(run)
	run.turns = append(run.turns, turnOutline{id: turnID, cardID: cardID})
	state.version++
	if cardID != 0 {
		o.card(cardID, CardOutline{RunID: runID, TurnID: turnID})
	}
	return turnID
}

// MarkTurnVisible records that a turn has observed visible assistant text or
// reasoning. Repeated calls are harmless.
func (o OutlineCards) MarkTurnVisible(turnID TurnID) bool {
	_, turn := o.turn(turnID)
	if turn == nil {
		return false
	}
	if !turn.visible {
		turn.visible = true
		o.state().version++
	}
	return true
}

// EndTurn records a completed turn boundary and its plain statistics. It starts
// the synthetic tool-batch window for this run.
func (o OutlineCards) EndTurn(turnID TurnID, stats TurnStats) bool {
	run, turn := o.turn(turnID)
	if run == nil || turn == nil || turn.ended || run.status != RunOpen {
		return false
	}
	turn.ended, turn.stats = true, stats
	o.state().version++
	if stats.DurationMs > 0 {
		run.llmDurationMs += stats.DurationMs
	}
	run.batchEligible = true
	run.currentBatch = 0
	return true
}

// AddTurnStat associates an already-appended statistic card with its completed
// turn, without changing the card's payload or renderer visibility policy.
func (o OutlineCards) AddTurnStat(turnID TurnID, cardID BlockID) bool {
	run, turn := o.turn(turnID)
	if run == nil || !turn.ended || cardID == 0 || o.conversation.index(cardID) < 0 {
		return false
	}
	if _, exists := o.Card(cardID); exists {
		return false
	}
	o.card(cardID, CardOutline{RunID: run.id, TurnID: turnID})
	return true
}

// AddActivity associates a non-turn, non-tool card observed inside an open run.
func (o OutlineCards) AddActivity(runID RunID, cardID BlockID) bool {
	if o.openRun(runID) == nil || cardID == 0 || o.conversation.index(cardID) < 0 {
		return false
	}
	if _, exists := o.Card(cardID); exists {
		return false
	}
	o.card(cardID, CardOutline{RunID: runID})
	return true
}

// AddTool records membership of a top-level tool card in an open run and assigns
// its deterministic prototype duration exactly once. Only tools after a turn
// end contribute to synthetic batch totals.
func (o OutlineCards) AddTool(runID RunID, cardID BlockID) (int64, bool) {
	run := o.openRun(runID)
	if run == nil || cardID == 0 || o.conversation.index(cardID) < 0 {
		return 0, false
	}
	if card, ok := o.Card(cardID); ok {
		return card.DurationMs, card.Tool && card.RunID == runID
	}
	duration := prototypeToolDuration(len(run.tools))
	if !run.batchEligible || run.currentBatch == 0 {
		run.nextBatch++
		run.currentBatch = run.nextBatch
	}
	run.tools = append(run.tools, toolOutline{cardID: cardID, durationMs: duration, batchID: run.currentBatch})
	o.card(cardID, CardOutline{RunID: runID, Tool: true, DurationMs: duration})
	if run.batchEligible {
		if duration > run.currentBatchMaxMs {
			run.currentBatchMaxMs = duration
		}
	}
	return duration, true
}

// SetReconnecting records whether an open run's live stream is reconnecting.
// Reconnecting is non-terminal and never closes the outline.
func (o OutlineCards) SetReconnecting(runID RunID, reconnecting bool) bool {
	run := o.openRun(runID)
	if run == nil {
		return false
	}
	if run.reconnecting != reconnecting {
		run.reconnecting = reconnecting
		o.state().version++
	}
	return true
}

// EndRun records a terminal result for an open run. status must be RunNormal,
// RunError, or RunInterrupted (server cancellation). Usage is authoritative only
// when supplied at this terminal boundary.
func (o OutlineCards) EndRun(runID RunID, status RunStatus, usage Usage) bool {
	if status != RunNormal && status != RunError && status != RunInterrupted {
		return false
	}
	run := o.openRun(runID)
	if run == nil {
		return false
	}
	closeBatch(run)
	run.status, run.usage, run.usageAvailable, run.reconnecting = status, usage, true, false
	if status == RunInterrupted && usage == (Usage{}) {
		// The client collapses missing and all-zero wire usage. A completed call
		// is evidence for known zero; cancellation before one is not.
		run.usageAvailable = false
		for _, turn := range run.turns {
			if turn.ended {
				run.usageAvailable = true
				break
			}
		}
	}
	o.state().version++
	return true
}

// Interrupt records that an open run ended without a terminal result.
func (o OutlineCards) Interrupt(runID RunID) bool {
	run := o.openRun(runID)
	if run == nil {
		return false
	}
	closeBatch(run)
	run.status, run.reconnecting = RunInterrupted, false
	o.state().version++
	return true
}

// Card returns detached structural metadata for cardID. It returns false for a
// card with no observed outline role.
func (o OutlineCards) Card(cardID BlockID) (CardOutline, bool) {
	if o.conversation.outline == nil {
		return CardOutline{}, false
	}
	card, ok := o.conversation.outline.cards[cardID]
	return card, ok
}

// Run returns a detached snapshot of an observed run.
func (o OutlineCards) Run(id RunID) (RunOutline, bool) {
	state := o.conversation.outline
	if state == nil {
		return RunOutline{}, false
	}
	for i := range state.runs {
		if state.runs[i].id == id {
			return snapshotRun(&state.runs[i]), true
		}
	}
	return RunOutline{}, false
}

// Runs returns detached snapshots in observed run order.
func (o OutlineCards) Runs() []RunOutline {
	state := o.conversation.outline
	if state == nil {
		return nil
	}
	out := make([]RunOutline, len(state.runs))
	for i := range state.runs {
		out[i] = snapshotRun(&state.runs[i])
	}
	return out
}

func (o OutlineCards) state() *outlineState {
	if o.conversation.outline == nil {
		o.conversation.outline = &outlineState{}
	}
	return o.conversation.outline
}

func (o OutlineCards) card(cardID BlockID, outline CardOutline) {
	state := o.state()
	if state.cards == nil {
		state.cards = make(map[BlockID]CardOutline)
	}
	state.cards[cardID] = outline
	state.version++
}

func (o OutlineCards) openRun(id RunID) *runOutline {
	state := o.conversation.outline
	if state == nil {
		return nil
	}
	for i := range state.runs {
		if state.runs[i].id == id && state.runs[i].status == RunOpen {
			return &state.runs[i]
		}
	}
	return nil
}

func (o OutlineCards) turn(id TurnID) (*runOutline, *turnOutline) {
	state := o.conversation.outline
	if state == nil {
		return nil, nil
	}
	for i := range state.runs {
		for j := range state.runs[i].turns {
			if state.runs[i].turns[j].id == id {
				return &state.runs[i], &state.runs[i].turns[j]
			}
		}
	}
	return nil, nil
}

func activeTurn(run *runOutline) *turnOutline {
	if len(run.turns) == 0 {
		return nil
	}
	turn := &run.turns[len(run.turns)-1]
	if turn.ended {
		return nil
	}
	return turn
}

func closeBatch(run *runOutline) {
	run.completedToolMs += run.currentBatchMaxMs
	run.currentBatchMaxMs = 0
	run.batchEligible = false
	run.currentBatch = 0
}

func snapshotRun(run *runOutline) RunOutline {
	out := RunOutline{
		ID: run.id, StartAfter: run.startAfter, Status: run.status, Usage: run.usage, UsageAvailable: run.usageAvailable, Reconnecting: run.reconnecting,
		LLMDurationMs: run.llmDurationMs, ToolDurationMs: run.completedToolMs + run.currentBatchMaxMs,
		Turns: make([]TurnOutline, len(run.turns)), Tools: make([]ToolOutline, len(run.tools)),
	}
	for i, turn := range run.turns {
		out.Turns[i] = TurnOutline{ID: turn.id, CardID: turn.cardID, Visible: turn.visible, Ended: turn.ended, Stats: turn.stats}
	}
	for i, tool := range run.tools {
		out.Tools[i] = ToolOutline{CardID: tool.cardID, DurationMs: tool.durationMs, BatchID: tool.batchID}
	}
	return out
}

// prototypeToolDuration is deliberately isolated prototype-only timing. It is
// deterministic and stored at tool observation time so rendering never invents
// or changes a duration. Replace it with engine-provided timing before release.
func prototypeToolDuration(toolOrdinal int) int64 {
	return 1000 + int64(toolOrdinal%4)*500
}
