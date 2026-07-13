# Vela Private Voting App

A confidential voting app built on [Vela CCE](https://github.com/HorizenOfficial/vela) — a platform that runs WebAssembly modules inside AWS Nitro Enclaves (TEE) with encrypted state on-chain.

**Privacy model:**
- Votes are private — only the voter sees their choice (encrypted P-521 event)
- Tallies are public — vote counts emitted as plaintext AppEvents on-chain
- Deanonymization — authorized auditors can request the full voter→choice map

Default proposals: `Yes / No / Abstain`

---

## How It Works

```
User submits encrypted vote payload
    → ProcessorEndpoint (on-chain queue)
    → Manager polls chain, loads encrypted state
    → Executor (TEE) decrypts state, runs WASM logic
    → WASM validates proposal + double-vote, updates tallies
    → Encrypted UserEvent emitted (voter's P-521 key)
    → Plaintext AppEvent emitted (public tally update)
    → New encrypted state committed on-chain
```

Your vote choice is never visible on-chain in plaintext.

---

## Prerequisites

- Docker Desktop
- Wallet CLI (`novaw-linux`) and configured `wallet/wallet.conf` — see [`docs/private-voting.md`](docs/private-voting.md)

### Start the local Vela stack

```bash
cd dockerfiles
cp .env.dev .env   # once
docker compose up -d
```

Verify it's up:
```bash
curl -s -X POST http://localhost:8545 -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","method":"eth_blockNumber","params":[],"id":1}'
```

---

## Build

```bash
cd voting-app
make build
# → voting-app/voting_app.wasm
```

---

## Deploy

```bash
cp voting-app/voting_app.wasm wallet/
cd wallet

docker run --rm --platform linux/amd64 -v $(pwd):/wallet -w /wallet \
  ubuntu:22.04 /wallet/novaw-linux deployapp \
  --wasm /wallet/voting_app.wasm \
  --max-value-fee "100 wei"
# → Deploy app completed successfully. ApplicationID: <number>
```

Copy the `ApplicationID` into `wallet/wallet.conf`.

---

## Interact

```bash
# Run from wallet/ directory
cd wallet

# Register your P-521 key on-chain (once per app)
docker run --rm --platform linux/amd64 -v $(pwd):/wallet -w /wallet \
  ubuntu:22.04 /wallet/novaw-linux registeruser
```

Cast a vote and read private events via the TypeScript client (`vela-common-ts`):

```ts
import { VelaClient } from 'vela-common-ts';

const client = new VelaClient({
  rpcUrl:           'http://localhost:8545',
  processorAddress: '0xCf7Ed3AccA5a467e9e704C703E8D87F634fB0Fc9',
  subgraphUrl:      'http://localhost:8000/subgraphs/name/hcce',
});

// Cast a vote (encrypts payload with TEE's P-521 key)
await client.submitRequest(APP_ID,
  JSON.stringify({ type: 'vote', vote: { proposal: 'Yes' } }),
  secp256k1PrivKey,
);

// Read your private confirmation event
const events = await client.getPrivateEvents(p521PrivKey, APP_ID);
// → [{ type: "vote_cast", proposal: "Yes", nonce: 1 }]
```

---

## Source Layout

```
voting-app/
  main.go        # WASM bridge (pointer conversions → app package)
  app/
    types.go     # VotingState, payload, event structs
    app.go       # Deploy, LoadModule, Deposit, ProcessRequest
  go.mod
  Makefile
docs/
  private-voting.md   # Full step-by-step tutorial
  3_typescript-client.md
dockerfiles/           # Local Vela stack (Anvil + Manager + Executor + Subgraph)
wallet/
  wallet.conf.template
```

---

## Full Tutorial

See [`docs/private-voting.md`](docs/private-voting.md) — covers state design, all WASM exports,
build, deploy, casting votes, reading events, and deanonymization.
