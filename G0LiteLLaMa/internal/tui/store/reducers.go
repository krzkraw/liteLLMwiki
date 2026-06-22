package store

import (
	"encoding/json"
)

// Action types for the UI domain.
const (
	ActionTypeSelectTab ActionType = "ui:select-tab"
)

// SelectTabPayload is the payload for a tab selection action.
type SelectTabPayload struct {
	TabID string `json:"tab_id"`
}

// EffectSpec represents scheduled side-effect work returned by a reducer.
// Reducers produce EffectSpec values but never execute them.
type EffectSpec struct {
	// Placeholder — expanded when effects are wired.
}

// Reducer transforms AppState in response to an ActionEnvelope.
// It must be deterministic and side-effect-free.
type Reducer func(AppState, ActionEnvelope) (AppState, []EffectSpec)

// RootReduce routes actions to domain reducers and manages cross-domain
// coordination. It always increments the state revision for accepted actions.
func RootReduce(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	state.Revision++

	switch action.Type {
	case ActionTypeSelectTab:
		return reduceSelectTab(state, action)
	default:
		return state, nil
	}
}

func reduceSelectTab(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	var p SelectTabPayload
	if err := json.Unmarshal(action.Payload, &p); err != nil {
		return state, nil
	}
	state.UI.ActiveTab = p.TabID
	return state, nil
}

// ReduceUI handles UI-domain actions.
func ReduceUI(state AppState, action ActionEnvelope) (AppState, []EffectSpec) {
	switch action.Type {
	case ActionTypeSelectTab:
		return reduceSelectTab(state, action)
	default:
		return state, nil
	}
}

// ReduceViewport is a pass-through reducer for the Viewport domain.
func ReduceViewport(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceRunners is a pass-through reducer for the Runners domain.
func ReduceRunners(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceModels is a pass-through reducer for the Models domain.
func ReduceModels(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceRuntime is a pass-through reducer for the Runtime domain.
func ReduceRuntime(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceChat is a pass-through reducer for the Chat domain.
func ReduceChat(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceWizard is a pass-through reducer for the Wizard domain.
func ReduceWizard(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}

// ReduceTasks is a pass-through reducer for the Tasks domain.
func ReduceTasks(state AppState, _ ActionEnvelope) (AppState, []EffectSpec) {
	return state, nil
}
