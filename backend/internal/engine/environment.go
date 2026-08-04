package engine

func (e *Engine) applyFog() {
	controlled := e.unit(e.moment.ControlledUnitID)
	if controlled == nil {
		return
	}
	for i := range e.session.Units {
		unit := &e.session.Units[i]
		unit.Visible = unit.Team == controlled.Team || distance(unit.Position, controlled.Position) <= 34
	}
}
