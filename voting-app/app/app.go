package app

import (
	"encoding/json"
	"fmt"

	"github.com/HorizenOfficial/vela-common-go/wasm/types"
	"github.com/HorizenOfficial/vela/pkg/common"
)

func Deploy(appId int64, paramsJSON string) types.DeployResult {
	proposals := defaultProposals
	if paramsJSON != "" {
		var params DeployParams
		if err := json.Unmarshal([]byte(paramsJSON), &params); err != nil {
			return types.DeployResult{Error: fmt.Sprintf("invalid deploy params: %v", err)}
		}
		if len(params.Proposals) > 0 {
			proposals = params.Proposals
		}
	}

	tallies := make(map[string]int64, len(proposals))
	for _, p := range proposals {
		tallies[p] = 0
	}

	state := VotingState{
		AppID:     uint64(appId),
		Proposals: proposals,
		Tallies:   tallies,
		HasVoted:  make(map[string]string),
	}
	stateJSON, _ := json.Marshal(state)
	return types.DeployResult{State: stateJSON, Fuel: types.NewUint256(5)}
}

// LoadModule is called on Executor restart for cache warm-up. Returns minimal state.
func LoadModule(appId int64) types.LoadModuleResult {
	state := VotingState{
		AppID:    uint64(appId),
		Tallies:  make(map[string]int64),
		HasVoted: make(map[string]string),
	}
	stateJSON, _ := json.Marshal(state)
	return types.LoadModuleResult{State: stateJSON, Fuel: types.NewUint256(5)}
}

// Deposit is a required export. This app does not use funds; state passes through unchanged.
func Deposit(senderPtr, tokenPtr *types.Address, value *types.Uint256, stateJSON string) types.DepositResult {
	return types.DepositResult{State: []byte(stateJSON), Fuel: types.NewUint256(0)}
}

func ProcessRequest(senderPtr *types.Address, requestType int32, payloadJSON, stateJSON string) types.ProcessResult {
	if senderPtr == nil {
		return types.ProcessResult{Error: "sender address is nil"}
	}
	sender := *senderPtr
	senderHex := sender.Hex()

	var state VotingState
	if err := json.Unmarshal([]byte(stateJSON), &state); err != nil {
		return types.ProcessResult{Error: fmt.Sprintf("failed to parse state: %v", err)}
	}

	// Deanonymize: return full voter→choice mapping to an authorized auditor.
	if requestType == int32(common.Deanonymize) {
		report, _ := json.Marshal(DeanonymizeReport{
			Votes:   state.HasVoted,
			Tallies: state.Tallies,
		})
		return types.ProcessResult{
			State:  []byte(stateJSON),
			Report: report,
			Fuel:   types.NewUint256(15),
		}
	}

	// Parse payload
	var payload PayloadInstructions
	if payloadJSON != "" && payloadJSON != "{}" {
		if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
			return types.ProcessResult{Error: fmt.Sprintf("invalid payload: %v", err)}
		}
	}

	if payload.Type != "vote" || payload.Vote == nil {
		return types.ProcessResult{Error: fmt.Sprintf("unsupported instruction type: %q", payload.Type)}
	}

	proposal := payload.Vote.Proposal

	// Validate proposal exists
	found := false
	for _, p := range state.Proposals {
		if p == proposal {
			found = true
			break
		}
	}
	if !found {
		return types.ProcessResult{Error: fmt.Sprintf("unknown proposal: %q", proposal)}
	}

	// One vote per address
	if _, alreadyVoted := state.HasVoted[senderHex]; alreadyVoted {
		return types.ProcessResult{Error: "address has already voted"}
	}

	// Record vote
	state.HasVoted[senderHex] = proposal
	state.Tallies[proposal]++
	state.Nonce++

	// Private event to voter (encrypted by Executor with voter's P-521 key)
	eventData, _ := json.Marshal(VoteCastEvent{
		Type:     "vote_cast",
		Proposal: proposal,
		Nonce:    state.Nonce,
	})

	// Public AppEvent: updated tally visible to anyone watching the chain
	tallyData, _ := json.Marshal(TallyUpdate{
		Proposal:   proposal,
		TotalVotes: state.Tallies[proposal],
	})

	newState, _ := json.Marshal(state)
	return types.ProcessResult{
		State:     newState,
		Events:    []types.PlainEvent{{UserID: sender, Data: eventData}},
		AppEvents: []types.AppEvent{{Data: tallyData}},
		Fuel:      types.NewUint256(30),
	}
}
