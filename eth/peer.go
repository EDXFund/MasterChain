// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package eth

import (
	"errors"
	"fmt"
	"github.com/EDXFund/MasterChain/core"
	"github.com/EDXFund/MasterChain/qchain"
	"github.com/EDXFund/MasterChain/rlp"
	"math/big"
	"sync"
	"time"

	"github.com/EDXFund/MasterChain/common"
	"github.com/EDXFund/MasterChain/core/types"
	"github.com/EDXFund/MasterChain/p2p"
	"github.com/deckarep/golang-set"
)

var (
	errClosed            = errors.New("peer set is closed")
	errAlreadyRegistered = errors.New("peer is already registered")
	errNotRegistered     = errors.New("peer is not registered")
	errEmpty             = errors.New("empty data")
)

const (
	maxKnownTxs    = 32768 // Maximum transactions hashes to keep in the known list (prevent DOS)
	maxKnownBlocks = 1024  // Maximum block hashes to keep in the known list (prevent DOS)

	// maxQueuedTxs is the maximum number of transaction lists to queue up before
	// dropping broadcasts. This is a sensitive number as a transaction list might
	// contain a single transaction, or thousands.
	maxQueuedTxs = 128000

	// maxQueuedProps is the maximum number of block propagations to queue up before
	// dropping broadcasts. There's not much point in queueing stale blocks, so a few
	// that might cover uncles should be enough.
	maxQueuedProps = 4

	// maxQueuedAnns is the maximum number of block announcements to queue up before
	// dropping broadcasts. Similarly to block propagations, there's no point to queue
	// above some healthy uncle limit, so use that.
	maxQueuedAnns = 4

	handshakeTimeout = 5 * time.Second
)

// PeerInfo represents a short summary of the Ethereum sub-protocol metadata known
// about a connected peer.
type PeerInfo struct {
	ShardId    uint16   `json:"shardId"`
	Version    int      `json:"version"`    // Ethereum protocol version negotiated
	Difficulty *big.Int `json:"difficulty"` // Total difficulty of the peer's blockchain
	Head       string   `json:"head"`       // SHA3 hash of the peer's best owned block
}

// propEvent is a block propagation, waiting for its turn in the broadcast queue.
type propEvent struct {
	block types.BlockIntf
	td    *big.Int
}

type peer struct {
	id      string
	shardId uint16
	*p2p.Peer
	rw p2p.MsgReadWriter

	version  int         // Protocol version negotiated
	forkDrop *time.Timer // Timed connection dropper if forks aren't validated in time

	head common.Hash
	td   *big.Int
	lock sync.RWMutex

	shardInfo []*types.SInfo

	knownTxs    mapset.Set                // Set of transaction hashes known to be known by this peer
	knownBlocks mapset.Set                // Set of block hashes known to be known by this peer
	queuedTxs   chan []*types.Transaction // Queue of transactions to broadcast to the peer
	queuedProps chan *propEvent           // Queue of blocks to broadcast to the peer
	//	queuedShardProps chan *propShardEvent // Queue of blocks to broadcast to the peer
	queuedAnns chan types.BlockIntf // Queue of blocks to announce to the peer
	//queuedShardAnns  chan *types.SBlock         // Queue of blocks to announce to the peer
	term chan struct{} // Termination channel to stop the broadcaster
}

func newPeer(version int, p *p2p.Peer, rw p2p.MsgReadWriter) *peer {
	return &peer{
		Peer: p,
		rw:   rw,

		version:     version,
		id:          fmt.Sprintf("%x", p.ID().Bytes()[:8]),
		knownTxs:    mapset.NewSet(),
		knownBlocks: mapset.NewSet(),
		queuedTxs:   make(chan []*types.Transaction, maxQueuedTxs),
		queuedProps: make(chan *propEvent, maxQueuedProps),
		queuedAnns:  make(chan types.BlockIntf, maxQueuedAnns),
		term:        make(chan struct{}),
	}
}

// broadcast is a write loop that multiplexes block propagations, announcements
// and transaction broadcasts into the remote peer. The goal is to have an async
// writer that does not lock up node internals.
func (p *peer) broadcast() {
	for {
		select {
		case txs := <-p.queuedTxs:
			if err := p.SendTransactions(txs); err != nil {
				return
			}
			p.Log().Trace("Broadcast transactions", "count", len(txs))

		case prop := <-p.queuedProps:
			if err := p.SendNewBlock(prop.block, prop.td); err != nil {
				return
			}
			p.Log().Trace("Propagated block", "number", prop.block.Number(), "hash", prop.block.Hash(), "td", prop.td)

		case block := <-p.queuedAnns:
			if err := p.SendNewBlockHashes(block.ShardId(), []common.Hash{block.Hash()}, []uint64{block.NumberU64()}); err != nil {
				return
			}
			p.Log().Trace("Announced block", "number", block.Number(), "hash", block.Hash())

		case <-p.term:
			return
		}
	}
}

// close signals the broadcast goroutine to terminate.
func (p *peer) close() {
	close(p.term)
}

// Info gathers and returns a collection of metadata known about a peer.
func (p *peer) Info() *PeerInfo {
	hash, td := p.Head(p.shardId)

	return &PeerInfo{
		ShardId:    p.shardId,
		Version:    p.version,
		Difficulty: td,
		Head:       hash.Hex(),
	}
}

// Head retrieves a copy of the current head hash and total difficulty of the
// peer.
//func (p *peer) Head() (hash common.Hash, td *big.Int) {
//	p.lock.RLock()
//	defer p.lock.RUnlock()
//
//	copy(hash[:], p.head[:])
//	return hash, new(big.Int).Set(p.td)
//}

// Head retrieves a copy of the current shard head hash and total difficulty of the
// peer.
func (p *peer) Head(shardId uint16) (hash common.Hash, td *big.Int) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, shard := range p.shardInfo {
		if shard.ShardId == shardId {
			copy(hash[:], shard.HeadHash[:])
			return hash, new(big.Int).Set(shard.Td)
		}
	}
	return common.Hash{}, big.NewInt(0)

}

//// SetHead updates the head hash and total difficulty of the peer.
//func (p *peer) SetHead(hash common.Hash, td *big.Int) {
//	p.lock.Lock()
//	defer p.lock.Unlock()
//
//	copy(p.head[:], hash[:])
//	p.td.Set(td)
//	//p.shardId = shardId
//}

func (p *peer) SetHead(hash common.Hash, td *big.Int, shardId uint16) {
	p.lock.RLock()
	defer p.lock.RUnlock()

	for _, shard := range p.shardInfo {
		if shard.ShardId == shardId {
			shard.HeadHash = hash
			shard.Td = td
		}
	}
	//p.shardId = shardId
}

// MarkBlock marks a block as known for the peer, ensuring that the block will
// never be propagated to this particular peer.
func (p *peer) MarkBlock(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known block hash
	//if p.shardId != types.ShardMaster && (shardId != p.shardId){
	//	return
	//}
	//
	for p.knownBlocks.Cardinality() >= maxKnownBlocks {
		p.knownBlocks.Pop()
	}
	p.knownBlocks.Add(hash)
}

// MarkTransaction marks a transaction as known for the peer, ensuring that it
// will never be propagated to this particular peer.
func (p *peer) MarkTransaction(hash common.Hash) {
	// If we reached the memory allowance, drop a previously known transaction hash
	for p.knownTxs.Cardinality() >= maxKnownTxs {
		p.knownTxs.Pop()
	}
	p.knownTxs.Add(hash)
}

// SendTransactions sends transactions to the peer and includes the hashes
// in its transaction hash set for future reference.
func (p *peer) SendTransactions(txs types.Transactions) error {
	for _, tx := range txs {
		p.knownTxs.Add(tx.Hash())
	}
	return p2p.Send(p.rw, TxMsg, txs)
}

// AsyncSendTransactions queues list of transactions propagation to a remote
// peer. If the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendTransactions(txs []*types.Transaction) {
	select {
	case p.queuedTxs <- txs:
		for _, tx := range txs {
			p.knownTxs.Add(tx.Hash())
		}
	default:
		p.Log().Debug("Dropping transaction propagation", "count", len(txs))
	}
}

// SendNewBlockHashes announces the availability of a number of blocks through
// a hash notification.
func (p *peer) SendNewBlockHashes(shardId uint16, hashes []common.Hash, numbers []uint64) error {
	for _, hash := range hashes {
		p.knownBlocks.Add(hash)
	}
	request := newBlockHashesData{ShardId: shardId}

	request.HashData = make([]hashesData, len(hashes))
	for i := 0; i < len(hashes); i++ {

		request.HashData[i].Hash = hashes[i]
		request.HashData[i].Number = numbers[i]
	}
	return p2p.Send(p.rw, NewBlockHashesMsg, request)
}

// AsyncSendNewBlockHash queues the availability of a block for propagation to a
// remote peer. If the peer's broadcast queue is full, the event is silently
// dropped.
func (p *peer) AsyncSendNewBlockHash(block types.BlockIntf) {
	select {
	case p.queuedAnns <- block:
		p.knownBlocks.Add(block.Hash())
	default:
		p.Log().Debug("Dropping block announcement", "number", block.NumberU64(), "hash", block.Hash())
	}
}

// SendNewBlock propagates an entire block to a remote peer.
func (p *peer) SendNewBlock(block types.BlockIntf, td *big.Int) error {
	p.knownBlocks.Add(block.Hash())
	msg := newBlockData{
		ShardId: block.ShardId(),
		TD:      td,
	}

	if msg.ShardId == types.ShardMaster {
		msg.Data, _ = rlp.EncodeToBytes(block.ToBlock())
	} else {
		msg.Data, _ = rlp.EncodeToBytes(block.ToSBlock())
	}

	return p2p.Send(p.rw, NewBlockMsg, msg)
}

// AsyncSendNewBlock queues an entire block for propagation to a remote peer. If
// the peer's broadcast queue is full, the event is silently dropped.
func (p *peer) AsyncSendNewBlock(block types.BlockIntf, td *big.Int) {
	select {
	case p.queuedProps <- &propEvent{block: block, td: td}:
		p.knownBlocks.Add(block.Hash())
	default:
		p.Log().Debug("Dropping block propagation", "number", block.NumberU64(), "hash", block.Hash())
	}
}

// SendBlockHeaders sends a batch of block headers to the remote peer.
func (p *peer) SendBlockHeaders(headers []types.HeaderIntf, shardId uint16) error {

	var data []byte

	if shardId == types.ShardMaster {
		hs := make([]*types.HeaderStruct, len(headers))
		if len(headers) > 0 {
			for key, val := range headers {
				hs[key] = val.ToHeader().ToHeaderStruct()
			}
		}
		data, _ = rlp.EncodeToBytes(hs)
	} else {
		hs := make([]*types.SHeaderStruct, len(headers))
		if len(headers) > 0 {
			for key, val := range headers {
				hs[key] = val.ToSHeader().ToStruct()
			}
		}
		data, _ = rlp.EncodeToBytes(hs)
	}

	msg := blockHeaderMsgData{ShardId: shardId, Data: data}
	return p2p.Send(p.rw, BlockHeadersMsg, msg)
}

// SendBlockBodies sends a batch of block contents to the remote peer.
func (p *peer) SendBlockBodies(bodies []types.BlockIntf, shardId uint16) error {
	lenBodies := len(bodies)
	if lenBodies > 0 {

		msg := blockBodiesData{ShardId: shardId}

		var err error
		if shardId == types.ShardMaster {
			data := make([]blockMasterBody, lenBodies)
			for i, val := range bodies {
				body := val.Body()
				data[i] = blockMasterBody{BlockInfos: body.ShardBlocks, Receipts: body.Receipts}
			}
			msg.Data, err = rlp.EncodeToBytes(data)
		} else {
			data := make([]blockShardBody, lenBodies)
			for i, val := range bodies {
				body := val.Body()
				data[i] = blockShardBody{Transactions: body.Transactions, ContractResults: body.Results}
			}
			msg.Data, err = rlp.EncodeToBytes(data)
		}
		if err == nil {
			return p2p.Send(p.rw, BlockBodiesMsg, msg)
		} else {
			return err
		}
	} else {
		return errEmpty
	}

}

// SendBlockBodiesRLP sends a batch of block contents to the remote peer from
// an already RLP encoded format.
func (p *peer) SendBlockBodiesRLP(bodies []rlp.RawValue) error {
	return p2p.Send(p.rw, BlockBodiesMsg, bodies)

}

// SendNodeDataRLP sends a batch of arbitrary internal data, corresponding to the
// hashes requested.
func (p *peer) SendNodeData(data [][]byte) error {
	return p2p.Send(p.rw, NodeDataMsg, data)
}

// SendReceiptsRLP sends a batch of transaction receipts, corresponding to the
// ones requested from an already RLP encoded format.
func (p *peer) SendReceiptsRLP(receipts []rlp.RawValue) error {
	return p2p.Send(p.rw, ReceiptsMsg, receipts)
}

// RequestOneHeader is a wrapper around the header query functions to fetch a
// single header. It is used solely by the fetcher.
func (p *peer) RequestOneHeader(hash common.Hash, shardId uint16) error {
	p.Log().Debug("Fetching single header", "hash", hash)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: hash}, Amount: uint64(1), Skip: uint64(0), Reverse: false, ShardId: shardId})
}

// RequestHeadersByHash fetches a batch of blocks' headers corresponding to the
// specified header query, based on the hash of an origin block.
func (p *peer) RequestHeadersByHash(origin common.Hash, amount int, skip int, reverse bool) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromhash", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{Origin: hashOrNumber{Hash: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestHeadersByNumber fetches a batch of blocks' headers corresponding to the
// specified header query, based on the number of an origin block.
func (p *peer) RequestHeadersByNumber(origin uint64, amount int, skip int, reverse bool, shardId uint16) error {
	p.Log().Debug("Fetching batch of headers", "count", amount, "fromnum", origin, "skip", skip, "reverse", reverse)
	return p2p.Send(p.rw, GetBlockHeadersMsg, &getBlockHeadersData{ShardId: shardId, Origin: hashOrNumber{Number: origin}, Amount: uint64(amount), Skip: uint64(skip), Reverse: reverse})
}

// RequestBodies fetches a batch of blocks' bodies corresponding to the hashes
// specified.
func (p *peer) RequestBodies(hashes []common.Hash, shardId uint16) error {
	p.Log().Debug("Fetching batch of block bodies", "count", len(hashes))
	gbd := getBlockBodysData{
		ShardId: shardId,
		Hashs:   hashes,
	}
	return p2p.Send(p.rw, GetBlockBodiesMsg, gbd)
}

// RequestNodeData fetches a batch of arbitrary data from a node's known state
// data, corresponding to the specified hashes.
func (p *peer) RequestNodeData(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of state data", "count", len(hashes))
	return p2p.Send(p.rw, GetNodeDataMsg, hashes)
}

// RequestReceipts fetches a batch of transaction receipts from a remote node.
func (p *peer) RequestReceipts(hashes []common.Hash) error {
	p.Log().Debug("Fetching batch of receipts", "count", len(hashes))
	return p2p.Send(p.rw, GetReceiptsMsg, hashes)
}

// Handshake executes the eth protocol handshake, negotiating version number,
// network IDs, difficulties, head and genesis blocks.
func (p *peer) Handshake(network uint64, blockChain *core.BlockChain, shardPool *qchain.ShardChainPool) error {
	// Send out own handshake in a new thread
	errc := make(chan error, 2)

	var (
		head   = blockChain.CurrentHeader()
		hash   = head.Hash()
		number = head.NumberU64()
		td     = blockChain.GetTd(hash, number)
	)

	var status statusData // safe to read after two values have been received from errc
	genesis := blockChain.Genesis()
	shardId := genesis.ShardId()
	genesisBlock := blockChain.GenesisHashOf(shardId)

	sInfo := []*types.SInfo{
		{
			ShardId:  shardId,
			Td:       td,
			HeadHash: hash,
		},
	}

	if shardId == types.ShardMaster {
		for shardId, shard := range shardPool.GetMaxTds() {
			sInfo = append(sInfo, &types.SInfo{
				ShardId:  shardId,
				Td:       new(big.Int).SetUint64(shard.Td),
				HeadHash: shard.Hash,
			})
		}
	}

	go func() {
		errc <- p2p.Send(p.rw, StatusMsg, &statusData{
			ProtocolVersion: uint32(p.version),
			NetworkId:       network,
			ShardId:         shardId,
			GenesisBlock:    genesisBlock,
			ShardInfo:       sInfo,
		})
	}()
	go func() {
		errc <- p.readStatus(network, &status, blockChain)
	}()
	timeout := time.NewTimer(handshakeTimeout)
	defer timeout.Stop()
	for i := 0; i < 2; i++ {
		select {
		case err := <-errc:
			if err != nil {
				return err
			}
		case <-timeout.C:
			return p2p.DiscReadTimeout
		}
	}
	p.shardInfo, p.shardId = status.ShardInfo, status.ShardId
	return nil
}

func (p *peer) readStatus(network uint64, status *statusData, blockChain *core.BlockChain) (err error) {
	msg, err := p.rw.ReadMsg()
	if err != nil {
		return err
	}
	if msg.Code != StatusMsg {
		return errResp(ErrNoStatusMsg, "first msg has code %x (!= %x)", msg.Code, StatusMsg)
	}
	if msg.Size > ProtocolMaxMsgSize {
		return errResp(ErrMsgTooLarge, "%v > %v", msg.Size, ProtocolMaxMsgSize)
	}
	// Decode the handshake and make sure everything matches
	if err := msg.Decode(&status); err != nil {
		return errResp(ErrDecode, "msg %v: %v", msg, err)
	}
	genesis := blockChain.GenesisHashOf(status.ShardId)
	if status.GenesisBlock != genesis {
		return errResp(ErrGenesisBlockMismatch, "%x (!= %x)", status.GenesisBlock[:8], genesis[:8])
	}
	if status.NetworkId != network {
		return errResp(ErrNetworkIdMismatch, "%d (!= %d)", status.NetworkId, network)
	}
	if int(status.ProtocolVersion) != p.version {
		return errResp(ErrProtocolVersionMismatch, "%d (!= %d)", status.ProtocolVersion, p.version)
	}
	return nil
}
func (ps *peer) ShardId() uint16 { return ps.shardId }

// String implements fmt.Stringer.
func (p *peer) String() string {
	return fmt.Sprintf("Peer %s [%s]", p.id,
		fmt.Sprintf("eth/%2d", p.version),
	)
}

type PeersOfShard map[string]*peer

// peerSet represents the collection of active peers currently participating in
// the Ethereum sub-protocol.
type peerSet struct {
	peers  map[uint16]PeersOfShard
	lock   sync.RWMutex
	closed bool
}

// newPeerSet creates a new peer set to track the active participants.
func newPeerSet() *peerSet {
	return &peerSet{
		peers: make(map[uint16]PeersOfShard),
	}
}

// Register injects a new peer into the working set, or returns an error if the
// peer is already known. If a new peer it registered, its broadcast loop is also
// started.
func (ps *peerSet) Register(p *peer) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	if ps.closed {
		return errClosed
	}
	if shardPeer, exist := ps.peers[p.shardId]; exist {
		if _, ok := shardPeer[p.id]; ok {
			return errAlreadyRegistered
		} else {
			shardPeer[p.id] = p
		}
	} else {
		ps.peers[p.shardId] = make(map[string]*peer)
		ps.peers[p.shardId][p.id] = p
	}

	go p.broadcast()

	return nil
}

// Unregister removes a remote peer from the active set, disabling any further
// actions to/from that particular entity.
func (ps *peerSet) Unregister(id string) error {
	ps.lock.Lock()
	defer ps.lock.Unlock()
	found := false
	//find in all shard
	for _, peers := range ps.peers {
		p, ok := peers[id]
		if ok {
			found = true
			delete(peers, id)
			p.close()
			break
		}
	}

	if !found {
		return errNotRegistered
	}
	return nil
}

// Peer retrieves the registered peer with the given id.
func (ps *peerSet) Peer(id string) *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	for _, peers := range ps.peers {
		p, ok := peers[id]
		if ok {
			return p
		}
	}
	return nil
}

// Len returns if the current number of peers in the set.
func (ps *peerSet) Len() int {
	ps.lock.RLock()
	defer ps.lock.RUnlock()
	alen := 0
	for _, peers := range ps.peers {
		alen += len(peers)
	}
	return alen
}

// PeersWithoutBlock retrieves a list of peers that do not have a given block in
// their set of known hashes.
//master block should broadcast to all peers
func (ps *peerSet) PeersWithoutMasterBlock(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, peers := range ps.peers {
		for _, p := range peers {
			if !p.knownBlocks.Contains(hash) {
				list = append(list, p)
			}
		}
	}
	return list
	//return ps.PeersWithoutShardBlock(types.ShardMaster,hash)
}
func (ps *peerSet) PeersWithoutShardBlock(shardId uint16, hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	if shardPeers, ok := ps.peers[shardId]; ok {
		for _, p := range shardPeers {
			if !p.knownBlocks.Contains(hash) {
				list = append(list, p)
			}
		}
	}

	return list
}

// PeersWithoutTx retrieves a list of peers that do not have a given transaction
// in their set of known hashes.
func (ps *peerSet) MasterPeersWithoutTx(hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	for _, p := range ps.peers[types.ShardMaster] {

		if !p.knownTxs.Contains(hash) {
			list = append(list, p)
		}
	}
	return list
}

func (ps *peerSet) ShardPeersWithoutTx(shardId uint16, hash common.Hash) []*peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	list := make([]*peer, 0, len(ps.peers))
	if shardPeers, ok := ps.peers[shardId]; ok {
		for _, p := range shardPeers {
			if !p.knownTxs.Contains(hash) {
				list = append(list, p)
			}
		}
	}
	return list
}

// BestPeer retrieves the known peer with the currently highest total difficulty.
func (ps *peerSet) BestPeer() *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *peer
		bestTd   *big.Int
	)
	if peers, ok := ps.peers[common.ShardMaster]; ok {
		for _, p := range peers {
			if _, td := p.Head(common.ShardMaster); bestPeer == nil || td.Cmp(bestTd) > 0 {
				bestPeer, bestTd = p, td
			}
		}
		return bestPeer
	} else {

		return nil
	}

}

// BestPeer retrieves the known peer with the currently highest total difficulty.
func (ps *peerSet) BestPeerOfShard(shardId uint16) *peer {
	ps.lock.RLock()
	defer ps.lock.RUnlock()

	var (
		bestPeer *peer
		bestTd   *big.Int
	)
	if shardPeers, ok := ps.peers[shardId]; ok {
		for _, p := range shardPeers {
			if _, td := p.Head(shardId); bestPeer == nil || td.Cmp(bestTd) > 0 {
				bestPeer, bestTd = p, td
			}
		}
		return bestPeer
	} else {
		return nil
	}

}

// Close disconnects all peers.
// No new peers can be registered after Close has returned.
func (ps *peerSet) Close() {
	ps.lock.Lock()
	defer ps.lock.Unlock()

	for _, p := range ps.peers {
		for _, ps := range p {
			ps.Disconnect(p2p.DiscQuitting)
		}

	}
	ps.closed = true
}
