# Building a Private Voting App on Vela (v0.1.0)

This tutorial builds a **confidential voting app** on Vela CCE from scratch. It is a simpler
alternative to the payment app and is a good first custom WASM app to write.

Privacy model:
- **Votes are private** — only the voter sees their own choice (via encrypted event)
- **Tallies are public** — vote counts are emitted as plaintext AppEvents visible to anyone
- **Deanonymization** — authorized auditors can request the full voter→choice mapping

> **Prerequisite**: Complete `steps.md` first. The Docker stack must be running and your
> `wallet/wallet.conf` must be configured (keys + contract addresses).

App source: `voting-app/` in this repository.

---

## What This App Does

| Operation | Who can see it | How |
|-----------|---------------|-----|
| Cast vote | Only the voter (encrypted event) | `process_request` with `{"type":"vote"}` |
| Running tally | Anyone (plaintext AppEvent on-chain) | Emitted automatically on each vote |
| Full voter map | Authorized auditor only | `deanonymize` request → encrypted report |

Each address can vote exactly once. Proposals are set at deploy time (default: `Yes / No / Abstain`).

---

## Project Structure

```
voting-app/
  go.mod          # Module deps: vela v0.1.0, vela-common-go v0.1.0
  Makefile        # Docker-based build targets
  main.go         # WASM bridge — thin exports layer, no business logic
  app/
    types.go      # State, payload, event structs
    app.go        # Business logic: Deploy, LoadModule, Deposit, ProcessRequest
```

---

## Step 1: Module Setup

### go.mod

```go
module github.com/example/vela-voting-app

go 1.24.0

require (
    github.com/HorizenOfficial/vela v0.1.0
    github.com/HorizenOfficial/vela-common-go v0.1.0
)
```

> `go mod tidy` runs inside `golang:1.24` as part of `make build` to satisfy the `vela-common-go`
> dependency requirement (Go >= 1.24) and generate `go.sum`. The `go` directive is then patched
> back to `1.23.0` so TinyGo 0.35.0 (which bundles Go 1.23) can compile it.

Two dependencies:
- `vela-common-go` — shared WASM types (`Address`, `Uint256`, `PlainEvent`, `AppEvent`, result structs)
- `vela` — request type constants (`common.Deanonymize = 2`) used for routing inside `process_request`

### Makefile

```makefile
build:
    docker run --rm --platform linux/amd64 \
      -v $(shell pwd):/app -w /app \
      golang:1.24 \
      sh -c "go mod tidy && sed -i 's/^go 1\.24.*/go 1.23.0/; /^toolchain /d' go.mod"
    docker run --rm --platform linux/amd64 \
      -v $(shell pwd):/app -w /app \
      tinygo/tinygo:0.35.0 \
      tinygo build -target=wasi -o voting_app.wasm .
```

Two stages because `vela-common-go v0.1.0` requires Go >= 1.24 (so `go mod tidy` must run with
a real Go 1.24 toolchain), but TinyGo 0.35.0 bundles Go 1.23 and rejects a `go 1.24` directive
in go.mod. The `sed` strips the `go 1.24` and `toolchain` lines written by tidy before handing
off to TinyGo.

---

## Step 2: State Design

All app state is JSON-serialized, then encrypted by the Executor (AES-256) and stored on-chain.
You receive it as a string on every call, mutate it in memory, and return the updated version.

```go
// VotingState is the encrypted app state persisted between requests.
type VotingState struct {
    AppID     uint64            `json:"appId"`
    Proposals []string          `json:"proposals"`
    Tallies   map[string]int64  `json:"tallies"`  // proposal -> vote count
    HasVoted  map[string]string `json:"hasVoted"` // voterHex -> proposal
    Nonce     uint64            `json:"nonce"`
}
```

Design choices:
- **`HasVoted` keyed by hex address** — enforces one-vote-per-address and is also the full
  voter list for deanonymization.
- **`Tallies` as `map[string]int64`** — simpler than `Uint256` since vote counts won't overflow int64.
- **No balances or tokens** — this app doesn't need funds; `deposit` is a required WASM export
  but is implemented as a pass-through.
- **`Nonce`** — included in events so voters can verify ordering.

### Payload types

```go
type PayloadInstructions struct {
    Type string           `json:"type"`
    Vote *VoteInstruction `json:"vote,omitempty"`
}

type VoteInstruction struct {
    Proposal string `json:"proposal"`
}
```

### Event types

```go
// VoteCastEvent — private, encrypted with voter's P-521 key
type VoteCastEvent struct {
    Type     string `json:"type"`     // "vote_cast"
    Proposal string `json:"proposal"` // what they voted for
    Nonce    uint64 `json:"nonce"`
}

// TallyUpdate — public AppEvent, plaintext, anyone can read
type TallyUpdate struct {
    Proposal   string `json:"proposal"`
    TotalVotes int64  `json:"totalVotes"`
}

// DeanonymizeReport — encrypted, only the requesting auditor can read
type DeanonymizeReport struct {
    Votes   map[string]string `json:"votes"`   // voterHex -> proposal
    Tallies map[string]int64  `json:"tallies"`
}
```

---

## Step 3: WASM Bridge (`main.go`)

`main.go` is a thin layer that converts raw WASM pointers into Go types and delegates to `app/`.
No business logic lives here.

```go
package main

import (
    "github.com/HorizenOfficial/vela-common-go/wasm/types"
    "github.com/HorizenOfficial/vela-common-go/wasm/utils"
    "github.com/example/vela-voting-app/app"
)

//export deploy
func deploy(appId int64, paramsPtr *byte, paramsLen int32) *byte {
    paramsJSON := utils.PtrToString(paramsPtr, paramsLen)
    return types.SerializeAndWriteResult(app.Deploy(appId, paramsJSON))
}

//export load_module
func load_module(appId int64) *byte {
    return types.SerializeAndWriteResult(app.LoadModule(appId))
}

//export deposit
func deposit(appId int64, senderPtr *byte, senderLen int32,
    tokenPtr *byte, tokenLen int32,
    valuePtr *byte, valueLen int32,
    statePtr *byte, stateLen int32) *byte {
    _ = appId
    sender := types.PtrToAddress(senderPtr, senderLen)
    token  := types.PtrToAddress(tokenPtr, tokenLen)
    value  := types.PtrToUint256(valuePtr, valueLen)
    stateJSON := utils.PtrToString(statePtr, stateLen)
    return types.SerializeAndWriteResult(app.Deposit(sender, token, value, stateJSON))
}

//export process_request
func process_request(appId int64, senderPtr *byte, senderLen int32,
    requestType int32,
    payloadPtr *byte, payloadLen int32,
    statePtr *byte, stateLen int32) *byte {
    _ = appId
    sender    := types.PtrToAddress(senderPtr, senderLen)
    payloadJSON := utils.PtrToString(payloadPtr, payloadLen)
    stateJSON   := utils.PtrToString(statePtr, stateLen)
    return types.SerializeAndWriteResult(app.ProcessRequest(sender, requestType, payloadJSON, stateJSON))
}

func main() {}
```

Key patterns:
- `//export <name>` — required TinyGo directive to expose the function to the host WASM runtime
- `utils.PtrToString` / `types.PtrToAddress` / `types.PtrToUint256` — convert WASM linear memory
  pointers (sent by the Executor) into Go values
- `types.SerializeAndWriteResult` — marshals your result struct to JSON, writes it into WASM memory
  with a 4-byte length prefix, and returns a pointer the Executor can read back
- `func main() {}` — required by Go; not called in WASM execution

---

## Step 4: Business Logic (`app/app.go`)

### Deploy — Initialize state

Called once when you run `deployapp`. Receives JSON constructor params and returns the initial state.

```go
func Deploy(appId int64, paramsJSON string) types.DeployResult {
    proposals := defaultProposals // ["Yes", "No", "Abstain"]
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
        AppID: uint64(appId), Proposals: proposals,
        Tallies: tallies, HasVoted: make(map[string]string),
    }
    stateJSON, _ := json.Marshal(state)
    return types.DeployResult{State: stateJSON, Fuel: types.NewUint256(5)}
}
```

### LoadModule — Cache warm-up

Called by the Executor on restart to rebuild its state cache. Returns a minimal empty state
(no proposals) — the real state for the deployed app is loaded from encrypted on-chain storage.

```go
func LoadModule(appId int64) types.LoadModuleResult {
    state := VotingState{
        AppID: uint64(appId),
        Tallies: make(map[string]int64), HasVoted: make(map[string]string),
    }
    stateJSON, _ := json.Marshal(state)
    return types.LoadModuleResult{State: stateJSON, Fuel: types.NewUint256(5)}
}
```

### Deposit — Pass-through

Required WASM export but this app doesn't use funds. Returns state unchanged.

```go
func Deposit(senderPtr, tokenPtr *types.Address, value *types.Uint256, stateJSON string) types.DepositResult {
    return types.DepositResult{State: []byte(stateJSON), Fuel: types.NewUint256(0)}
}
```

### ProcessRequest — Vote and Deanonymize

```go
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

    // Deanonymize: return full voter->choice map to an authorized auditor
    if requestType == int32(common.Deanonymize) {
        report, _ := json.Marshal(DeanonymizeReport{
            Votes: state.HasVoted, Tallies: state.Tallies,
        })
        return types.ProcessResult{State: []byte(stateJSON), Report: report, Fuel: types.NewUint256(15)}
    }

    // Parse vote payload
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

    // Validate proposal
    found := false
    for _, p := range state.Proposals {
        if p == proposal { found = true; break }
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

    // Private event (encrypted by Executor with voter's P-521 key)
    eventData, _ := json.Marshal(VoteCastEvent{
        Type: "vote_cast", Proposal: proposal, Nonce: state.Nonce,
    })
    // Public AppEvent (plaintext — anyone watching the chain sees the tally update)
    tallyData, _ := json.Marshal(TallyUpdate{
        Proposal: proposal, TotalVotes: state.Tallies[proposal],
    })

    newState, _ := json.Marshal(state)
    return types.ProcessResult{
        State:     newState,
        Events:    []types.PlainEvent{{UserID: sender, Data: eventData}},
        AppEvents: []types.AppEvent{{Data: tallyData}},
        Fuel:      types.NewUint256(30),
    }
}
```

**Fuel values summary:**

| Operation | Fuel | Why |
|-----------|------|-----|
| `deploy` / `load_module` | 5 | Minimal computation |
| `deposit` | 0 | Pass-through, no work done |
| `vote` | 30 | Map lookups, JSON serialization, event emission |
| `deanonymize` | 15 | Read-only report generation |

---

## Step 5: Build

From the `voting-app/` directory:

```bash
cd voting-app
make build
```

On success you'll see `voting_app.wasm` in `voting-app/`.

**Common errors:**

| Error | Fix |
|-------|-----|
| `missing go.sum entry` | `go mod tidy` is part of `make build` — make sure Docker has internet access |
| `requires go version X through Y, got goZ` | TinyGo 0.35.0 bundles Go 1.23 and rejects `go 1.24` in go.mod; the two-stage Makefile patches this automatically |
| `exec format error` | Missing `--platform linux/amd64` in docker run |
| `no such file or directory` (voting_app.wasm) | Build hasn't run yet or failed — check `make build` output |
| `cannot use ... as type` | Type mismatch between your struct and vela-common-go's types |

---

## Step 6: Deploy

Copy the WASM binary to your wallet folder and deploy:

```bash
cp voting-app/voting_app.wasm wallet/
cd wallet

docker run --rm --platform linux/amd64 -v $(pwd):/wallet -w /wallet \
  ubuntu:22.04 /wallet/novaw-linux deployapp \
  --wasm /wallet/voting_app.wasm \
  --max-value-fee "100 wei"
```

On success:
```
Deploy app completed successfully. ApplicationID: <number>
```

Update `wallet/wallet.conf` — replace the existing `ApplicationID` with the new value:
```ini
ApplicationID=<new number from deployapp output>
```

> **Note on proposals**: The default proposals (`Yes`, `No`, `Abstain`) are hardcoded in `app/types.go`.
> If you want custom proposals, pass `--constructor-params '{"proposals":["Alice","Bob"]}'` to
> `deployapp` if your wallet CLI version supports that flag.

---

## Step 7: Register and Cast a Vote

### Register your P-521 key (required once per app)

```bash
cd wallet

docker run --rm --platform linux/amd64 -v $(pwd):/wallet -w /wallet \
  ubuntu:22.04 /wallet/novaw-linux registeruser
```

### Cast a vote (TypeScript client)

The wallet CLI (`novaw-linux`) is built for the reference payment app and has no generic
`submitrequest` command. To submit a custom encrypted payload, use `VelaClient` from
[`vela-common-ts`](https://github.com/HorizenOfficial/vela-common-ts).

**Install:**
```bash
npm install vela-common-ts
```

**Cast a vote:**
```ts
import { VelaClient } from 'vela-common-ts';

const client = new VelaClient({
  rpcUrl:           'http://localhost:8545',
  processorAddress: '0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9',
  subgraphUrl:      'http://localhost:8000/subgraphs/name/hcce',
});

const APP_ID = BigInt('<your ApplicationID from deployapp>');

// Encrypt and submit a vote
await client.submitRequest(
  APP_ID,
  JSON.stringify({ type: 'vote', vote: { proposal: 'Yes' } }),
  '<your secp256k1 private key hex>',  // pays gas
  '<TEE P-521 public key>',            // from TeeAuthenticatorAddress contract
);
```

> See `docs/3_typescript-client.md` for the full `VelaClient` API, key derivation, and how
> to read the `TeeAuthenticatorAddress` contract for the TEE's P-521 public key.

---

## Step 8: Read Your Private Event

After your vote is processed (watch for `RequestCompleted` on-chain), decrypt your event:

```ts
const events = await client.getPrivateEvents(
  '<your P-521 private key hex>',
  APP_ID,
);

// Each event decrypts to:
// { type: "vote_cast", proposal: "Yes", nonce: 1 }
console.log(events);
```

### Read the public tally

AppEvents are emitted as plaintext `AppEvent(applicationId, requestId, eventSubType, data)` logs.
Query the subgraph or listen on-chain:

```ts
// Subgraph query (example)
const query = `{
  appEvents(where: { applicationId: "<APP_ID>" }) {
    data   # JSON: { "proposal": "Yes", "totalVotes": 3 }
  }
}`;
```

---

## Step 9: Verify Double-Vote Protection

Submitting a second vote from the same address returns an error in `RequestCompleted`:

```
address has already voted
```

The on-chain state is not modified and no fee is charged (beyond gas).

---

## Step 10: Deanonymize (Authority Audit)

Only addresses registered in the `AuthorityRegistry` contract can submit deanonymization requests.
The Executor encrypts the report with the authority's P-521 key; the Authority Service exposes
it via HTTP.

```ts
await client.submitDeanonymizationRequest(APP_ID, secp256k1Key);
// Retrieve report via Authority Service:
// GET http://localhost:8081/report/<requestId>
// Decrypts to: { votes: { "0xabc...": "Yes", "0xdef...": "No" }, tallies: { "Yes": 5, ... } }
```

---

## What's Happening Under the Hood

When you cast a vote:

1. Client encrypts `{"type":"vote","vote":{"proposal":"Yes"}}` with the TEE's P-521 key
2. `submitRequest` is called on-chain — request lands in the FIFO queue
3. Manager polls the queue, loads the encrypted state from LevelDB, and sends everything to the Executor
4. Executor decrypts state (AES-256), calls `process_request` in the WASM module
5. WASM logic validates proposal + double-vote, updates `HasVoted` and `Tallies`, builds two outputs:
   - `PlainEvent` for the voter (encrypted by Executor with voter's P-521 key)
   - `AppEvent` tally update (plaintext)
6. Executor re-encrypts the updated state, signs a payload, sends it to the Manager
7. Manager calls `stateUpdate` on-chain — events emitted, state root updated
8. Voter decrypts their `UserEvent` with their P-521 private key to confirm the vote

The vote choice is never visible on-chain in plaintext. Only the voter and an authorized auditor
(via deanonymization) can see who voted for what.

---

## Summary

Building on Vela CCE comes down to:

1. **Define state** — a JSON struct holding everything the app needs between calls
2. **Implement 4 exports** — `deploy`, `load_module`, `deposit`, `process_request`
3. **Emit events** — `PlainEvent` for per-user private data; `AppEvent` for public signals
4. **Return fuel** — proportional to compute cost
5. **Compile with TinyGo** — `tinygo build -target=wasi .`
6. **Deploy** — `deployapp` uploads to Authority Service and registers on-chain

The platform handles all cryptography. Your WASM module works with plaintext data.
