package mediatedtransfer

import (
	"encoding/gob"

	"math/big"

	"github.com/SmartMeshFoundation/SmartRaiden/channel/channeltype"
	"github.com/SmartMeshFoundation/SmartRaiden/encoding"
	"github.com/SmartMeshFoundation/SmartRaiden/network/rpc/contracts"
	"github.com/SmartMeshFoundation/SmartRaiden/transfer"
	"github.com/SmartMeshFoundation/SmartRaiden/transfer/mtree"
	"github.com/SmartMeshFoundation/SmartRaiden/transfer/route"
	"github.com/ethereum/go-ethereum/common"
)

/*
ActionInitInitiatorStateChange start a mediated transfer
 Note: The init states must contain all the required data for trying doing
 useful work, ie. there must /not/ be an event for requesting new data.
*/
type ActionInitInitiatorStateChange struct {
	OurAddress     common.Address       //This node address.
	Tranfer        *LockedTransferState //A state object containing the transfer details.
	Routes         *route.RoutesState   //The current available routes.
	BlockNumber    int64                //The current block number.
	Db             channeltype.Db       //get the latest channel state
	LockSecretHash common.Hash
	Secret         common.Hash
}

//ActionInitMediatorStateChange  Initial state for a new mediator.
type ActionInitMediatorStateChange struct {
	OurAddress  common.Address             //This node address.
	FromTranfer *LockedTransferState       //The received MediatedTransfer.
	Routes      *route.RoutesState         //The current available routes.
	FromRoute   *route.State               //The route from which the MediatedTransfer was received.
	BlockNumber int64                      //The current block number.
	Message     *encoding.MediatedTransfer //the message trigger this statechange
	Db          channeltype.Db             //get the latest channel state
}

type MediatorReReceiveStateChange struct {
	Message *encoding.MediatedTransfer //it two message
}

//ActionInitTargetStateChange Initial state for a new target.
type ActionInitTargetStateChange struct {
	OurAddress  common.Address       //This node address.
	FromTranfer *LockedTransferState //The received MediatedTransfer.
	FromRoute   *route.State         //The route from which the MediatedTransfer was received.
	BlockNumber int64
	Message     *encoding.MediatedTransfer //the message trigger this statechange
	Db          channeltype.Db             //get the latest channel state
}

/*
ActionCancelRouteStateChange Cancel the current route.
 Notes:
        Used to cancel a specific route but not the transfer, may be used for
        timeouts.
*/
type ActionCancelRouteStateChange struct {
	LockSecretHash common.Hash
}

//ReceiveSecretRequestStateChange A SecretRequest message received.
type ReceiveSecretRequestStateChange struct {
	Amount         *big.Int
	LockSecretHash common.Hash
	Sender         common.Address
	Message        *encoding.SecretRequest //the message trigger this statechange
}

//ReceiveSecretRevealStateChange A SecretReveal message received
type ReceiveSecretRevealStateChange struct {
	Secret  common.Hash
	Sender  common.Address
	Message *encoding.RevealSecret //the message trigger this statechange
}

//ReceiveAnnounceDisposedStateChange A AnnounceDisposed message received.
type ReceiveAnnounceDisposedStateChange struct {
	Sender  common.Address
	Lock    *mtree.Lock
	Token   common.Address
	Message *encoding.AnnounceDisposed //the message trigger this statechange
}

//ReceiveBalanceProofStateChange A balance proof `identifier` was received.
type ReceiveBalanceProofStateChange struct {
	LockSecretHash common.Hash
	NodeAddress    common.Address
	BalanceProof   *transfer.BalanceProofState
	Message        encoding.EnvelopMessager //the message trigger this statechange
}

/*
warn  所有的合约事件都应该是按照链上发生的顺序抵达,这样可以保证同一个通道 settle 重新打开以后,不至于把事件发送给错误的通道.
*/
type ContractStateChange interface {
	GetBlockNumber() int64
}

/*
密码在链上注册了
1.诚实的节点在检查对方可以在链上unlock 这个锁的时候,应该主动发送unloc消息,移除此锁
2.自己应该把密码保存在本地,然后在需要的时候链上兑现
*/
type ContractSecretRevealStateChange struct {
	Secret      common.Hash
	BlockNumber int64
}

func (e *ContractSecretRevealStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}

type ContractUnlockStateChange struct {
	ChannelIdentifier   common.Hash
	BlockNumber         int64
	TokenNetworkAddress common.Address
	LockSecretHash      common.Hash
	Participant         common.Address
	TransferAmount      *big.Int
}

func (e *ContractUnlockStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}

type ContractChannelWithdrawStateChange struct {
	ChannelAddress *contracts.ChannelUniqueID
	//剩余的 balance 有意义?目前提供的 Event 并不知道 Participant1是谁,所以没啥用.
	Participant1        common.Address
	Participant1Balance *big.Int
	Participant2        common.Address
	Participant2Balance *big.Int
	TokenNetworkAddress common.Address
	BlockNumber         int64
}

func (e *ContractChannelWithdrawStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}

//ContractClosedStateChange a channel was closed
type ContractClosedStateChange struct {
	ChannelIdentifier   common.Hash
	ClosingAddress      common.Address
	ClosedBlock         int64 //block number when close
	LocksRoot           common.Hash
	TransferredAmount   *big.Int
	TokenNetworkAddress common.Address
	BlockNumber         int64
}

func (e *ContractClosedStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}

//ContractSettledStateChange a channel was settled
type ContractSettledStateChange struct {
	ChannelIdentifier   common.Hash
	SettledBlock        int64
	TokenNetworkAddress common.Address
}

func (e *ContractSettledStateChange) GetBlockNumber() int64 {
	return e.SettledBlock
}

//ContractCooperativeSettledStateChange a channel was cooperatively settled
type ContractCooperativeSettledStateChange struct {
	ChannelIdentifier   common.Hash
	SettledBlock        int64
	TokenNetworkAddress common.Address
}

func (e *ContractCooperativeSettledStateChange) GetBlockNumber() int64 {
	return e.SettledBlock
}

//ContractBalanceStateChange new deposit on channel
type ContractBalanceStateChange struct {
	ChannelIdentifier   common.Hash
	ParticipantAddress  common.Address
	Balance             *big.Int
	TokenNetworkAddress common.Address
	BlockNumber         int64
}

func (e *ContractBalanceStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}

//ContractNewChannelStateChange new channel created on block chain
type ContractNewChannelStateChange struct {
	ChannelIdentifier   *contracts.ChannelUniqueID
	Participant1        common.Address
	Participant2        common.Address
	SettleTimeout       int
	TokenNetworkAddress common.Address
	BlockNumber         int64
}

func (e *ContractNewChannelStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}

//ContractTokenAddedStateChange a new token registered
type ContractTokenAddedStateChange struct {
	RegistryAddress     common.Address
	TokenAddress        common.Address
	TokenNetworkAddress common.Address
	BlockNumber         int64
}

func (e *ContractTokenAddedStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}

//ContractBalanceProofUpdatedStateChange contrct TransferUpdated event
type ContractBalanceProofUpdatedStateChange struct {
	ChannelIdentifier   common.Hash
	Participant         common.Address
	LocksRoot           common.Hash
	TransferAmount      *big.Int
	TokenNetworkAddress common.Address
	BlockNumber         int64
}

func (e *ContractBalanceProofUpdatedStateChange) GetBlockNumber() int64 {
	return e.BlockNumber
}
func init() {
	gob.Register(&ActionInitInitiatorStateChange{})
	gob.Register(&ActionInitMediatorStateChange{})
	gob.Register(&ActionInitTargetStateChange{})
	gob.Register(&ActionCancelRouteStateChange{})
	gob.Register(&ReceiveSecretRequestStateChange{})
	gob.Register(&ReceiveSecretRevealStateChange{})
	gob.Register(&ReceiveAnnounceDisposedStateChange{})
	gob.Register(&ReceiveBalanceProofStateChange{})
	gob.Register(&ContractSecretRevealStateChange{})
	gob.Register(&ContractClosedStateChange{})
	gob.Register(&ContractSettledStateChange{})
	gob.Register(&ContractBalanceStateChange{})
	gob.Register(&ContractNewChannelStateChange{})
	gob.Register(&ContractTokenAddedStateChange{})
	gob.Register(&ContractBalanceProofUpdatedStateChange{})
}
