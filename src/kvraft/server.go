package kvraft

import (
	"lab5/labgob"
	"lab5/labrpc"
	"lab5/raft"
	"sync"
	"time"
)


type Op struct {
	Command   string
	ClientId  int64
	RequestId int64
	Key       string
	Value     string
}


type KVServer struct {
	mu       sync.Mutex
	me       int
	rf       *raft.Raft
	applyCh  chan raft.ApplyMsg
	data     map[string]string       // Key-value store
	ack      map[int64]int64         // Tracks latest RequestId per ClientId
	resultCh map[int]chan Result     // Log index → result channel
}

type Result struct {
	Command     string
	OK          bool
	ClientId    int64
	RequestId   int64
	WrongLeader bool
	Err         Err
	Value       string
}


func (kv *KVServer) appendEntry(entry Op) Result {
	// Start the entry via Raft
	index, _, isLeader := kv.rf.Start(entry)
	if !isLeader {
		return Result{OK: false}
	}

	// Ensure there's a channel to receive the result
	var ch chan Result
	kv.mu.Lock()
	if _, ok := kv.resultCh[index]; !ok {
		kv.resultCh[index] = make(chan Result, 1)
	}
	ch = kv.resultCh[index]
	kv.mu.Unlock()

	// Wait for result or timeout
	// Reduced timeout from 80ms to 20ms to speed up retries on a wrong leader.
	timer := time.NewTimer(80 * time.Millisecond)
	defer timer.Stop()

	select {
	case res := <-ch:
		if res.ClientId == entry.ClientId && res.RequestId == entry.RequestId {
			return res
		}
		return Result{OK: false}
	case <-timer.C:
		return Result{OK: false}
	}
}

func (kv *KVServer) Get(args *GetArgs, reply *GetReply) {
	op := Op{
		Command:   "get",
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		Key:       args.Key,
	}

	res := kv.appendEntry(op)

	if res.OK {
		reply.WrongLeader = false
		reply.Err = res.Err
		reply.Value = res.Value
	} else {
		reply.WrongLeader = true
	}
}

func (kv *KVServer) PutAppend(args *PutAppendArgs, reply *PutAppendReply) {
	op := Op{
		Command:   args.Command,
		ClientId:  args.ClientId,
		RequestId: args.RequestId,
		Key:       args.Key,
		Value:     args.Value,
	}

	res := kv.appendEntry(op)

	if res.OK {
		reply.WrongLeader = false
		reply.Err = res.Err
	} else {
		reply.WrongLeader = true
	}
}

// the tester calls Kill() when a KVServer instance won't
// be needed again. for your convenience, we supply
// code to set rf.dead (without needing a lock),
// and a killed() method to test rf.dead in
// long-running loops. you can also add your own
// code to Kill(). you're not required to do anything
// about this, but it may be convenient (for example)
// to suppress debug output from a Kill()ed instance.
func (kv *KVServer) Kill() {
	kv.rf.Kill()
}


func (kv *KVServer) applyOp(op Op) Result {
	res := Result{
		Command:   op.Command,
		OK:        true,
		ClientId:  op.ClientId,
		RequestId: op.RequestId,
	}

	switch op.Command {
	case "put":
		if !kv.isDuplicated(op) {
			kv.data[op.Key] = op.Value
		}
		res.Err = OK

	case "append":
		if !kv.isDuplicated(op) {
			kv.data[op.Key] += op.Value
		}
		res.Err = OK

	case "get":
		if val, exists := kv.data[op.Key]; exists {
			res.Value = val
			res.Err = OK
		} else {
			res.Err = ErrNoKey
		}
	}

	// Record the most recent request ID for deduplication
	kv.ack[op.ClientId] = op.RequestId
	return res
}

func (kv *KVServer) isDuplicated(op Op) bool {
	if lastId, ok := kv.ack[op.ClientId]; ok {
		return op.RequestId <= lastId
	}
	return false
}


func (kv *KVServer) Run() {
	for msg := range kv.applyCh {
		if !msg.CommandValid {
			continue
		}

		op := msg.Command.(Op)

		kv.mu.Lock()
		result := kv.applyOp(op)

		ch, exists := kv.resultCh[msg.CommandIndex]
		if !exists {
			ch = make(chan Result, 1)
			kv.resultCh[msg.CommandIndex] = ch
		}

		select {
		case <-ch: // drain old result if present
		default:
		}
		
		ch <- result
		go func(index int) {
			time.Sleep(200 * time.Millisecond)
			kv.mu.Lock()
			delete(kv.resultCh, index)
			kv.mu.Unlock()
		}(msg.CommandIndex)
		
		kv.mu.Unlock()
	}
}

func StartKVServer(servers []*labrpc.ClientEnd, me int, persister *raft.Persister, maxraftstate int) *KVServer {
	// call labgob.Register on structures you want
	// Go's RPC library to marshall/unmarshall.
	labgob.Register(Op{})
	labgob.Register(Result{})

	kv := &KVServer{
		me:       me,
		applyCh:  make(chan raft.ApplyMsg, 100),
		data:     make(map[string]string),
		ack:      make(map[int64]int64),
		resultCh: make(map[int]chan Result),
	}

	kv.rf = raft.Make(servers, me, persister, kv.applyCh)

	go kv.Run()
	return kv
}
