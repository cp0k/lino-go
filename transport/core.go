// Package transport implements the functionalities that
// directly interact with the blockchain to query data or broadcast transaction.
package transport

import (
	"context"
	"fmt"
	"time"

	"github.com/cosmos/cosmos-sdk/wire"
	"github.com/lino-network/lino-go/errors"
	"github.com/lino-network/lino-go/model"
	"github.com/spf13/viper"

	sdk "github.com/cosmos/cosmos-sdk/types"
	cmn "github.com/tendermint/tendermint/libs/common"
	rpcclient "github.com/tendermint/tendermint/rpc/client"
	ctypes "github.com/tendermint/tendermint/rpc/core/types"
)

// Transport is a wrapper of tendermint rpc client and codec.
type Transport struct {
	chainId         string
	nodeUrl         string
	client          rpcclient.Client
	Cdc             *wire.Codec
	queryTimeout    time.Duration
	BroadcastOkChan chan bool
	QueryOkChan     chan bool
}

// NewTransportFromConfig initiates an instance of Transport from config files.
func NewTransportFromConfig(queryTimeout time.Duration) *Transport {
	v := viper.New()
	viper.SetConfigType("json")
	v.SetConfigName("config")
	v.AddConfigPath("$HOME/.lino-go/")
	v.AutomaticEnv()
	v.ReadInConfig()

	nodeUrl := v.GetString("node_url")
	if nodeUrl == "" {
		nodeUrl = "localhost:26657"
	}
	rpc := rpcclient.NewHTTP(nodeUrl, "/websocket")
	return &Transport{
		chainId:         v.GetString("chain_id"),
		nodeUrl:         nodeUrl,
		client:          rpc,
		Cdc:             MakeCodec(),
		queryTimeout:    queryTimeout,
		BroadcastOkChan: make(chan bool),
		QueryOkChan:     make(chan bool),
	}
}

// NewTransportFromArgs initiates an instance of Transport from parameters passed in.
func NewTransportFromArgs(chainID, nodeUrl string, queryTimeout time.Duration) *Transport {
	if nodeUrl == "" {
		nodeUrl = "localhost:26657"
	}
	rpc := rpcclient.NewHTTP(nodeUrl, "/websocket")
	return &Transport{
		chainId:         chainID,
		nodeUrl:         nodeUrl,
		client:          rpc,
		Cdc:             MakeCodec(),
		queryTimeout:    queryTimeout,
		BroadcastOkChan: make(chan bool),
		QueryOkChan:     make(chan bool),
	}
}

// Query from Tendermint with the provided key and storename
func (t Transport) Query(key cmn.HexBytes, storeName string) (res []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), t.queryTimeout)
	defer cancel()

	go func() {
		res, err = t.query(key, storeName, "key", 0)
	}()

	select {
	case <-t.QueryOkChan:
		break
	case <-ctx.Done():
		return nil, errors.Timeout("query timeout").AddCause(ctx.Err())
	}

	return res, err
}

// Query from Tendermint with the provided key and storename at certain height
func (t Transport) QueryAtHeight(key cmn.HexBytes, storeName string, height int64) (res []byte, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), t.queryTimeout)
	defer cancel()

	go func() {
		res, err = t.query(key, storeName, "key", height)
	}()

	select {
	case <-t.QueryOkChan:
		break
	case <-ctx.Done():
		return nil, errors.Timeoutf("query at height %v timeout", height).AddCause(ctx.Err())
	}

	return res, err
}

// Query from Tendermint with the provided subspace and storename
func (t Transport) QuerySubspace(subspace []byte, storeName string) (res []sdk.KVPair, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), t.queryTimeout)
	defer cancel()

	var resRaw []byte
	go func() {
		resRaw, err = t.query(subspace, storeName, "subspace", 0)
	}()

	select {
	case <-t.QueryOkChan:
		break
	case <-ctx.Done():
		return nil, errors.Timeout("query subspace timeout").AddCause(ctx.Err())
	}

	if err != nil {
		return nil, err
	}

	t.Cdc.UnmarshalJSON(resRaw, &res)
	return
}

func (t Transport) query(key cmn.HexBytes, storeName, endPath string, height int64) (res []byte, err error) {
	path := fmt.Sprintf("/store/%s/%s", storeName, endPath)
	node, err := t.GetNode()
	if err != nil {
		return res, err
	}

	opts := rpcclient.ABCIQueryOptions{
		Height:  height,
		Trusted: true,
	}
	result, err := node.ABCIQueryWithOptions(path, key, opts)
	if err != nil {
		return res, err
	}

	resp := result.Response
	if resp.Code != uint32(0) {
		return res, errors.QueryFail("Query failed").AddBlockChainCode(resp.Code).AddBlockChainLog(resp.Log)
	}

	if resp.Value == nil || len(resp.Value) == 0 {
		return nil, errors.EmptyResponse("Empty response!")
	}

	t.QueryOkChan <- true

	return resp.Value, nil
}

// QueryBlock queries a block with a certain height from blockchain.
func (t Transport) QueryBlock(height int64) (res *ctypes.ResultBlock, err error) {
	node, err := t.GetNode()
	if err != nil {
		return res, err
	}

	return node.Block(&height)
}

// QueryBlockStatus queries block status from blockchain.
func (t Transport) QueryBlockStatus() (res *ctypes.ResultStatus, err error) {
	node, err := t.GetNode()
	if err != nil {
		return res, err
	}

	return node.Status()
}

// BroadcastTx broadcasts a transcation to blockchain.
func (t Transport) BroadcastTx(tx []byte) (*ctypes.ResultBroadcastTxCommit, error) {
	node, err := t.GetNode()
	if err != nil {
		return nil, err
	}

	res, err := node.BroadcastTxCommit(tx)
	if err != nil {
		return nil, err
	}

	t.BroadcastOkChan <- true

	return res, nil
}

// SignBuildBroadcast signs msg with private key and then broadcasts
// the transaction to blockchain.
func (t Transport) SignBuildBroadcast(msg model.Msg,
	privKeyHex string, seq int64, memo string) (*ctypes.ResultBroadcastTxCommit, error) {
	msgs := []model.Msg{msg}

	privKey, err := GetPrivKeyFromHex(privKeyHex)
	if err != nil {
		return nil, err
	}

	signMsgBytes, err := EncodeSignMsg(t.Cdc, msgs, t.chainId, seq)
	if err != nil {
		return nil, err
	}
	// SignatureFromBytes
	sig, err := privKey.Sign(signMsgBytes)
	if err != nil {
		return nil, err
	}

	// build transaction bytes
	txByte, err := EncodeTx(t.Cdc, msgs, privKey.PubKey(), sig, seq, memo)
	if err != nil {
		return nil, err
	}

	// broadcast
	return t.BroadcastTx(txByte)
}

// GetNote returns the Tendermint rpc client node.
func (t Transport) GetNode() (rpcclient.Client, error) {
	if t.client == nil {
		return nil, errors.InvalidNodeURL("Must define node URL")
	}
	return t.client, nil
}
