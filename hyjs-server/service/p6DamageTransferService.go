package service

// The original reduction-transfer Buff belongs to the target but also modifies
// its source. Keep the same identity at both ends so suppression/unload cannot
// leave a source bonus behind or duplicate it on re-Effect.
func (e *combatRoundEngine) attachReductionTransfer(b *combatRoundBuff) {
	b.source.outgoingReduction = append(b.source.outgoingReduction, b)
}
func (e *combatRoundEngine) detachReductionTransfer(b *combatRoundBuff) {
	for i, current := range b.source.outgoingReduction {
		if current == b {
			b.source.outgoingReduction = append(b.source.outgoingReduction[:i], b.source.outgoingReduction[i+1:]...)
			return
		}
	}
}
func (e *combatRoundEngine) clearBothCampsOnceCast(sourceCamp int32) {
	for _, camp := range []int32{sourceCamp, 1 - sourceCamp} {
		for _, actor := range e.players[camp].actors {
			e.clearOnce(actor, false)
		}
	}
}
