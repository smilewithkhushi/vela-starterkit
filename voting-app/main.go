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
	token := types.PtrToAddress(tokenPtr, tokenLen)
	value := types.PtrToUint256(valuePtr, valueLen)
	stateJSON := utils.PtrToString(statePtr, stateLen)
	return types.SerializeAndWriteResult(app.Deposit(sender, token, value, stateJSON))
}

//export process_request
func process_request(appId int64, senderPtr *byte, senderLen int32,
	requestType int32,
	payloadPtr *byte, payloadLen int32,
	statePtr *byte, stateLen int32) *byte {
	_ = appId
	sender := types.PtrToAddress(senderPtr, senderLen)
	payloadJSON := utils.PtrToString(payloadPtr, payloadLen)
	stateJSON := utils.PtrToString(statePtr, stateLen)
	return types.SerializeAndWriteResult(app.ProcessRequest(sender, requestType, payloadJSON, stateJSON))
}

func main() {}
