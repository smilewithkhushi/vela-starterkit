package app

// VotingState is the encrypted app state persisted between requests.
type VotingState struct {
	AppID     uint64            `json:"appId"`
	Proposals []string          `json:"proposals"`
	Tallies   map[string]int64  `json:"tallies"`  // proposal -> vote count
	HasVoted  map[string]string `json:"hasVoted"` // voterHex -> proposal
	Nonce     uint64            `json:"nonce"`
}

// DeployParams are the constructor parameters passed at deploy time.
type DeployParams struct {
	Proposals []string `json:"proposals"`
}

// VoteInstruction is the payload for casting a vote.
type VoteInstruction struct {
	Proposal string `json:"proposal"`
}

// PayloadInstructions is the top-level encrypted request payload.
type PayloadInstructions struct {
	Type string           `json:"type"`
	Vote *VoteInstruction `json:"vote,omitempty"`
}

// VoteCastEvent is the private event data sent back to the voter (encrypted).
type VoteCastEvent struct {
	Type     string `json:"type"`
	Proposal string `json:"proposal"`
	Nonce    uint64 `json:"nonce"`
}

// TallyUpdate is the public AppEvent data emitted on each vote (plaintext).
type TallyUpdate struct {
	Proposal   string `json:"proposal"`
	TotalVotes int64  `json:"totalVotes"`
}

// DeanonymizeReport is returned to authorized auditors.
type DeanonymizeReport struct {
	Votes   map[string]string `json:"votes"`   // voterHex -> proposal
	Tallies map[string]int64  `json:"tallies"`
}

var defaultProposals = []string{"Yes", "No", "Abstain"}
