package raiden_network

import (
	"errors"
	"sync"

	"fmt"

	"time"

	"github.com/SmartMeshFoundation/raiden-network/channel"
	"github.com/SmartMeshFoundation/raiden-network/network"
	"github.com/SmartMeshFoundation/raiden-network/params"
	"github.com/SmartMeshFoundation/raiden-network/transfer"
	"github.com/SmartMeshFoundation/raiden-network/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/fatedier/frp/src/utils/log"
)

type ConnectionManager struct {
	BOOTSTRAP_ADDR      common.Address //class member
	raiden              *RaidenService
	api                 *RaidenApi
	lock                sync.Mutex
	channelGraph        *network.ChannelGraph
	tokenAddress        common.Address
	funds               int64
	initChannelTarget   int64
	joinableFundsTarget float64
}

func NewConnectionManager(raiden *RaidenService, tokenAddress common.Address, graph *network.ChannelGraph) *ConnectionManager {
	cm := &ConnectionManager{
		raiden:              raiden,
		api:                 NewRaidenApi(raiden),
		channelGraph:        graph,
		tokenAddress:        tokenAddress,
		funds:               0,
		initChannelTarget:   3,
		joinableFundsTarget: 0.4,
	}
	cm.BOOTSTRAP_ADDR = common.HexToAddress("0x0202020202020202020202020202020202020202")
	return cm
}

/*
Connect to the network.
        Use this to establish a connection with the token network.

        Subsequent calls to `connect` are allowed, but will only affect the spendable
        funds and the connection strategy parameters for the future. `connect` will not
        close any channels.

        Note: the ConnectionManager does not discriminate manually opened channels from
        automatically opened ones. If the user manually opened channels, those deposit
        amounts will affect the funding per channel and the number of new channels opened.

        Args:
            funds (int): the amount of tokens spendable for this
            ConnectionManager.
            initial_channel_target (int): number of channels to open immediately
            joinable_funds_target (float): amount of funds not initially assigned
*/
func (this *ConnectionManager) Connect(funds, initialChannelTarget int64, joinableFundsTarget float64) error {
	if funds <= 0 {
		return errors.New("connecting needs a positive value for `funds`")
	}
	_, ok := this.raiden.MessageHandler.blockedTokens[this.tokenAddress]
	if ok {
		delete(this.raiden.MessageHandler.blockedTokens, this.tokenAddress)
	}
	this.initChannelTarget = initialChannelTarget
	this.joinableFundsTarget = joinableFundsTarget
	openChannels := this.openChannels()
	if len(openChannels) > 0 {
		log.Debug(fmt.Sprintf("connect() called on an already joined token network tokenaddress=%s,openchannels=%d,sumdeposits=%d,funds=%d", utils.APex(this.tokenAddress), len(openChannels), this.sumDeposits(), this.funds))
	}
	if !this.channelGraph.HaveNodes() {
		log.Debug("bootstrapping token network.")
		this.lock.Lock()
		_, err := this.api.Open(this.tokenAddress, this.BOOTSTRAP_ADDR, this.raiden.Config.SettleTimeout, this.raiden.Config.RevealTimeout)
		if err != nil {
			log.Error(fmt.Sprint("open channel between %s and %s error:%s", utils.APex(this.tokenAddress), utils.APex(this.BOOTSTRAP_ADDR), err))
		}
		this.lock.Unlock()
	}
	this.lock.Lock()
	this.funds = funds
	err := this.addNewPartners()
	this.lock.Unlock()
	return err
}
func (this *ConnectionManager) openChannels() []*channel.Channel {
	chs := this.api.GetChannelList(this.tokenAddress, utils.EmptyAddress)
	var chs2 []*channel.Channel
	for _, c := range chs {
		if c.State() == transfer.CHANNEL_STATE_OPENED {
			chs2 = append(chs2, c)
		}
	}
	return chs2
}

//"The calculated funding per partner depending on configuration and
//overall funding of the ConnectionManager.
func (this *ConnectionManager) initialFundingPerPartner() int64 {
	if this.initChannelTarget > 0 {
		var f1, f2 float64
		f1 = float64(this.funds)
		f2 = float64(this.initChannelTarget)
		return int64(f1 * (1 - this.joinableFundsTarget) / f2)
	}
	return 0
}

/*
True, if funds available and the `initial_channel_target` was not yet
        reached.
*/
func (this *ConnectionManager) WantsMoreChannels() bool {
	_, ok := this.raiden.MessageHandler.blockedTokens[this.tokenAddress]
	if ok {
		return false
	}
	return this.fundsRemaining() > 0 && len(this.openChannels()) < int(this.initChannelTarget)
}

//The remaining funds after subtracting the already deposited amounts.
func (this *ConnectionManager) fundsRemaining() int64 {
	if this.funds > 0 {
		remaining := this.funds - this.sumDeposits()
		return remaining
	}
	return 0
}

//Shorthand for getting sum of all open channels deposited funds
func (this *ConnectionManager) sumDeposits() int64 {
	chs := this.openChannels()
	var sum int64 = 0
	for _, c := range chs {
		sum += c.ContractBalance()
	}
	return sum
}

//Shorthand for getting channels that had received any transfers in this token network
func (this *ConnectionManager) receivingChannels() (chs []*channel.Channel) {
	for _, c := range this.openChannels() {
		if len(c.ReceivedTransfers) > 0 {
			chs = append(chs, c)
		}
	}
	return
}

//Returns the minimum necessary waiting time to settle all channels.
func (this *ConnectionManager) minSettleBlocks() int64 {
	chs := this.receivingChannels()
	var maxTimeout int64 = -1
	currentBlock := this.raiden.GetBlockNumber()
	for _, c := range chs {
		var sinceClosed int64
		if c.State() == transfer.CHANNEL_STATE_CLOSED {
			sinceClosed = currentBlock - c.ExternState.ClosedBlock
		} else if c.State() == transfer.CHANNEL_STATE_OPENED {
			sinceClosed = -1
		} else {
			sinceClosed = 0
		}
		t := int64(c.SettleTimeout) - sinceClosed
		if maxTimeout < t {
			maxTimeout = t
		}
	}
	return maxTimeout
}
func (this *ConnectionManager) LeaveState() bool {
	_, ok := this.raiden.MessageHandler.blockedTokens[this.tokenAddress]
	return ok || this.initChannelTarget < 1
}

/*
Close all channels in the token network.
        Note: By default we're just discarding all channels we haven't received anything.
        This potentially leaves deposits locked in channels after `closing`. This is "safe"
        from an accounting point of view (deposits can not be lost), but may still be
        undesirable from a liquidity point of view (deposits will only be freed after
        manually closing or after the partner closed the channel).

        If only_receiving is False then we close and settle all channels irrespective of them
        having received transfers or not.
*/
func (this *ConnectionManager) closeAll(onlyReceiving bool) []*channel.Channel {
	this.lock.Lock()
	defer this.lock.Unlock()
	this.initChannelTarget = 0
	var channelsToClose []*channel.Channel
	if onlyReceiving {
		channelsToClose = this.receivingChannels()
	} else {
		channelsToClose = this.openChannels()
	}
	for _, c := range channelsToClose {
		_, err := this.api.Close(this.tokenAddress, c.PartnerState.Address)
		if err != nil {
			log.Error("close channel %s error:%s", utils.APex(c.MyAddress), err)
		}
	}
	return channelsToClose
}
func (this *ConnectionManager) LeaveAsync() *network.AsyncResult {
	result := network.NewAsyncResult()
	go func() {
		this.Leave(true)
		result.Result <- nil
		close(result.Result)
	}()
	return result
}

/*
Leave the token network.
        This implies closing all channels and waiting for all channels to be settled.
*/
func (this *ConnectionManager) Leave(onlyReceiving bool) []*channel.Channel {
	this.raiden.MessageHandler.blockedTokens[this.tokenAddress] = true
	if this.initChannelTarget > 0 {
		this.initChannelTarget = 0
	}
	closedChannels := this.closeAll(onlyReceiving)
	this.WaitForSettle(closedChannels)
	return closedChannels
}

/*
"Wait for all closed channels of the token network to settle.
        Note, that this does not time out.
*/
func (this *ConnectionManager) WaitForSettle(closedChannels []*channel.Channel) bool {
	found := false
	for {
		for _, c := range closedChannels {
			if c.State() != transfer.CHANNEL_STATE_SETTLED {
				found = true
				break
			}
		}
		if found {
			time.Sleep(time.Minute)
		} else {
			break
		}
	}
	return true
}

/*
Open a channel with `partner` and deposit `funding_amount` tokens.

        If the channel was already opened (a known race condition),
        this skips the opening and only deposits.
*/
func (this *ConnectionManager) openAndDeposit(partner common.Address, fundingAmount int64) error {
	_, err := this.api.Open(this.tokenAddress, partner, this.raiden.Config.SettleTimeout, this.raiden.Config.RevealTimeout)
	if err != nil {
		return err
	}
	cg := this.raiden.Token2ChannelGraph[this.tokenAddress]
	_, ok := cg.PartenerAddress2Channel[partner]
	if !ok {
		err = fmt.Errorf("Opening new channel failed; channel already opened,  but partner not in channelgraph ,partner=%s,tokenaddress=%s", utils.APex(partner), utils.APex(this.tokenAddress))
		log.Error(err.Error())
		return err
	} else {
		err = this.api.Deposit(this.tokenAddress, partner, fundingAmount, params.DEFAULT_POLL_TIMEOUT)
		if err != nil {
			log.Error(err.Error())
		}
		return err
	}
}

/*
This opens channels with a number of new partners according to the
        connection strategy parameter `self.initial_channel_target`.
        Each new channel will receive `self.initial_funding_per_partner` funding.
*/
func (this *ConnectionManager) addNewPartners() error {
	newPartnerCount := int(this.initChannelTarget) - len(this.openChannels())
	if newPartnerCount <= 0 {
		return nil
	}
	for _, partner := range this.findNewPartners(newPartnerCount) {
		err := this.openAndDeposit(partner, this.initialFundingPerPartner())
		if err != nil {
			log.Error(fmt.Sprintf("addNewPartners %s ,err:%s", utils.APex(partner), err))
			return err
		}
	}
	return nil
}

/*
Will be called when new channels in the token network are detected.
        If the minimum number of channels was not yet established, it will try
        to open new channels.

        If the connection manager has no funds, this is a noop.
*/
func (this *ConnectionManager) RetryConnect() {
	if this.funds <= 0 {
		return
	}
	if this.LeaveState() {
		return
	}
	this.lock.Lock()
	defer this.lock.Unlock()
	if this.fundsRemaining() <= 0 {
		return
	}
	if len(this.openChannels()) >= int(this.initChannelTarget) {
		return
	}
	//try to fullfill our connection goal
	this.addNewPartners()
}

/*
Will be called, when we were selected as channel partner by another
        node. It will fund the channel with up to the partner's deposit, but
        not more than remaining funds or the initial funding per channel.

        If the connection manager has no funds, this is a noop.
*/
func (this *ConnectionManager) JoinChannel(partnerAddress common.Address, partnerDepost int64) {
	if this.funds <= 0 {
		return
	}
	if this.LeaveState() {
		return
	}
	this.lock.Lock()
	defer this.lock.Unlock()
	remaining := this.fundsRemaining()
	initial := this.initialFundingPerPartner()
	joiningFunds := partnerDepost
	if joiningFunds > remaining {
		joiningFunds = remaining
	}
	if joiningFunds > initial {
		joiningFunds = initial
	}
	if joiningFunds <= 0 {
		return
	}
	err := this.api.Deposit(this.tokenAddress, partnerAddress, joiningFunds, params.DEFAULT_POLL_TIMEOUT)
	log.Debug("joined a channel funds=%d,me=%s,partner=%s err=%s", joiningFunds, utils.APex(this.raiden.NodeAddress), utils.APex(partnerAddress), err)
	return
}

/*
Search the token network for potential channel partners.

        Args:
            number (int): number of partners to return
*/
func (this *ConnectionManager) findNewPartners(number int) []common.Address {
	var known = make(map[common.Address]bool)
	for _, c := range this.openChannels() {
		known[c.PartnerState.Address] = true
	}
	known[this.BOOTSTRAP_ADDR] = true
	known[this.raiden.NodeAddress] = true
	allnodes := this.channelGraph.AllNodes()
	var availables []common.Address
	for _, n := range allnodes {
		if !known[n] {
			availables = append(availables, n)
		}
	}
	log.Debug(fmt.Sprintf("found %d partners", len(availables)))
	if number < len(availables) {
		return availables[:number]
	} else {
		return availables
	}

}
