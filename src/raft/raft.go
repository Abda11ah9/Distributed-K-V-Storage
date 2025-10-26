package raft

import (
	"bytes"
	"fmt"
	"lab5/constants"
	"lab5/labgob"
	"lab5/labrpc"
	"lab5/logger"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

type ApplyMsg struct {
	CommandValid bool
	Command      interface{}
	CommandIndex int
}

const (
	Follower = iota
	Candidate
	Leader
)

const (
	MinElectionTimeout = 300 * time.Millisecond
	MaxElectionTimeout = 600 * time.Millisecond
	HeartbeatInterval  = 120 * time.Millisecond
)

type LogEntry struct {
	Term    int
	Command interface{}
}

// A Go object implementing a single Raft peer
type Raft struct {
	mu        sync.Mutex          // Lock to protect shared access to this peer's state
	peers     []*labrpc.ClientEnd // RPC end points of all peers
	persister *Persister          // Object to hold this peer's persisted state
	me        int                 // this peer's index into peers[]
	dead      int32               // set by Kill()

	currentTerm int
	votedFor    int
	log         []LogEntry

	commitIndex int
	lastApplied int
	nextIndex   []int
	matchIndex  []int

	logger *logger.Logger
	// Your data here (4A, 4B, 4C).
	// Look at the paper's Figure 2 for a description of what
	// state a Raft server must maintain.
	state      int
	totalVotes int

	electionInterval time.Duration

	applyCh     chan ApplyMsg
	heartbeat   chan bool
	voted       chan bool
	newLeaderCh chan bool
}

// return currentTerm and whether this server
// believes it is the leader.
func (rf *Raft) GetState() (int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	term := rf.currentTerm
	isleader := rf.state == Leader

	return term, isleader
}

// save Raft's persistent state to stable storage,
// where it can later be retrieved after a crash and restart.
// see paper's Figure 2 for a description of what should be persistent.
// before you've implemented snapshots, you should pass nil as the
// second argument to persister.Save().
// after you've implemented snapshots, pass the current snapshot
// (or nil if there's not yet a snapshot).
func (rf *Raft) persist() {
	w := new(bytes.Buffer)
	e := labgob.NewEncoder(w)
	e.Encode(rf.currentTerm)
	e.Encode(rf.votedFor)
	e.Encode(rf.log)
	raftstate := w.Bytes()
	rf.persister.Save(raftstate, nil)
}

// restore previously persisted state.
func (rf *Raft) readPersist(data []byte) {
	if data == nil || len(data) < 1 {
		return
	}
	r := bytes.NewBuffer(data)
	d := labgob.NewDecoder(r)
	d.Decode(&rf.currentTerm)
	d.Decode(&rf.votedFor)
	d.Decode(&rf.log)
}

func (rf *Raft) getLastLogIndex() int {
	return len(rf.log) - 1
}

func (rf *Raft) getLastLogTerm() int {
	if len(rf.log) == 0 {
		return 0
	}
	return rf.log[len(rf.log)-1].Term
}

type RequestVoteArgs struct {
	Term        int
	CandidateId int
	LastLogIdx  int
	LogTerm     int
}
type AppendEntriesArgs struct {
	Term         int
	LeaderId     int
	PrevLogIndex int
	PrevLogTerm  int
	Entries      []LogEntry
	LeaderCommit int
}

type AppendEntriesReply struct {
	Term    int
	Success bool
	NextTry int
}
type RequestVoteReply struct {
	Term        int
	VoteGranted bool
}

func (rf *Raft) isUptoDate(cIndex int, cTerm int) bool {
	term, index := rf.getLastLogTerm(), rf.getLastLogIndex()
	if cTerm != term {
		return cTerm >= term
	}
	return cIndex >= index
}
func (rf *Raft) RequestVote(args *RequestVoteArgs, reply *RequestVoteReply) {
	// Your code here (4A, 4B).
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	reply.VoteGranted = false

	// Previous term ignore it
	if args.Term < rf.currentTerm {
		return
	}

	if args.Term > rf.currentTerm {
		rf.currentTerm = args.Term
		rf.state = Follower
		rf.votedFor = -1
	}

	reply.Term = rf.currentTerm
	if (rf.votedFor == -1 || rf.votedFor == args.CandidateId) &&
		rf.isUptoDate(args.LastLogIdx, args.LogTerm) {
		reply.VoteGranted = true
		rf.votedFor = args.CandidateId
		go func() { // using thread to avoid deadlocks
			// signal that you elected a new leader, should reset election timeout
			rf.voted <- true
		}()
	}
}

func (rf *Raft) applyLogs() {
	rf.mu.Lock()
	defer rf.mu.Unlock()

	for i := rf.lastApplied + 1; i <= rf.commitIndex; i++ {
		rf.applyCh <- ApplyMsg{CommandValid: true, Command: rf.log[i].Command, CommandIndex: i}
	}
	rf.lastApplied = rf.commitIndex
}
func (rf *Raft) getTermFirstAppearance(index int, term int) int {
	for index > 0 && rf.log[index].Term == term {
		index--
	}
	return index + 1
}
func (rf *Raft) AppendEntries(args *AppendEntriesArgs, reply *AppendEntriesReply) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	defer rf.persist()

	reply.Success = false

	if args.Term < rf.currentTerm {
		reply.Term = rf.currentTerm
		reply.Success = false
		return
	}
	//if args.Term > rf.currentTerm {
	//	rf.currentTerm = args.Term
	//	rf.status = Follower
	//	rf.votedFor = -1
	//}
	rf.state = Follower
	rf.currentTerm = args.Term
	rf.heartbeat <- true
	//TODO: part 2B
	reply.Term = rf.currentTerm

	if args.PrevLogIndex > rf.getLastLogIndex() {
		reply.Success = false
		reply.NextTry = rf.getLastLogIndex() + 1
		return
	}

	if args.PrevLogIndex >= 0 && rf.log[args.PrevLogIndex].Term != args.PrevLogTerm {
		reply.Success = false
		term := rf.log[args.PrevLogIndex].Term
		reply.NextTry = rf.getTermFirstAppearance(args.PrevLogIndex-1, term)
	} else {
		rf.log = append(rf.log[:args.PrevLogIndex+1], args.Entries...)
		reply.Success = true

		if args.LeaderCommit > rf.commitIndex {
			rf.commitIndex = min(args.LeaderCommit, len(rf.log)-1)
			go rf.applyLogs()
		}
	}
	return
}

func (rf *Raft) commitEntries() {

	for entry := rf.getLastLogIndex(); entry > rf.commitIndex; entry-- {
		// 		Leader can only commit entries of its current term.
		committed := 1
		majority := len(rf.peers) / 2
		if rf.log[entry].Term == rf.currentTerm {
			for server := range rf.peers {
				if rf.matchIndex[server] >= entry && server != rf.me { // This server has committed this far.
					committed++
				}
			}
		}
		if committed > majority {
			rf.commitIndex = entry
			go rf.applyLogs()
			break
		}
	}

}

func (rf *Raft) sendHeartBeat(server int, args *AppendEntriesArgs, reply *AppendEntriesReply) bool {
	ok := rf.peers[server].Call("Raft.AppendEntries", args, reply)
	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		if rf.state == Leader && args.Term == rf.currentTerm {
			if reply.Term > rf.currentTerm {
				rf.currentTerm = reply.Term
				rf.state = Follower
				rf.votedFor = -1
				return ok
			}
			if reply.Success {
				rf.matchIndex[server] = args.PrevLogIndex + len(args.Entries)
				rf.nextIndex[server] = rf.matchIndex[server] + 1
			} else {
				rf.nextIndex[server] = reply.NextTry
			}

			rf.commitEntries()
		}
	}
	return ok
}

func (rf *Raft) sendHeartBeats() {

	rf.mu.Lock()
	defer rf.mu.Unlock()

	for i := range rf.peers {
		if i != rf.me && rf.state == Leader {
			args := &AppendEntriesArgs{}
			args.Term = rf.currentTerm
			args.LeaderId = rf.me
			args.PrevLogIndex = rf.nextIndex[i] - 1
			args.LeaderCommit = rf.commitIndex
			if args.PrevLogIndex >= 0 {
				args.PrevLogTerm = rf.log[args.PrevLogIndex].Term
			}
			if rf.nextIndex[i] <= rf.getLastLogIndex() {
				args.Entries = rf.log[rf.nextIndex[i]:]
			}
			go rf.sendHeartBeat(i, args, &AppendEntriesReply{})
		}
	}

}
func (rf *Raft) startElection() {

	rf.mu.Lock()
	rf.currentTerm++
	rf.votedFor = rf.me
	rf.totalVotes = 1
	rf.persist()

	// Prepare vote request arguments.
	args := RequestVoteArgs{
		Term:        rf.currentTerm,
		CandidateId: rf.me,
		LastLogIdx:  rf.getLastLogIndex(),
		LogTerm:     rf.getLastLogTerm(),
	}
	rf.mu.Unlock()
	for i := range rf.peers {
		if i != rf.me && rf.state == Candidate {
			var replyArgs RequestVoteReply
			go rf.getAllRequestVotes(i, &args, &replyArgs)
		}
	}

}

func (rf *Raft) getAllRequestVotes(server int, args *RequestVoteArgs, reply *RequestVoteReply) {
	//ok := rf.sendRequestVote(server, args, reply)
	ok := rf.peers[server].Call("Raft.RequestVote", args, reply)
	if ok {
		rf.mu.Lock()
		defer rf.mu.Unlock()
		if rf.state == Candidate && args.Term == rf.currentTerm {
			if reply.Term > rf.currentTerm {
				// There's another leader
				rf.currentTerm = reply.Term
				rf.votedFor = -1
				rf.state = Follower
				rf.persist()
				return
			}
			if reply.VoteGranted {
				rf.totalVotes++
				majority := len(rf.peers)/2 + 1
				if rf.totalVotes >= majority {
					//rf.state = Leader
					go func() {
						rf.newLeaderCh <- true
					}()
				}
			}
		}

	}
	return
}

func (rf *Raft) Kill() {
	atomic.StoreInt32(&rf.dead, 1)
	// Your code here, if desired.
}

func (rf *Raft) killed() bool {
	z := atomic.LoadInt32(&rf.dead)
	return z == 1
}

func (rf *Raft) ticker() {
	for rf.killed() == false {
		switch rf.state {
		case Leader:
			rf.sendHeartBeats()
			time.Sleep(HeartbeatInterval)

		case Follower:
			select {
			case <-rf.voted:
			case <-rf.heartbeat:
			case <-time.After(MinElectionTimeout + time.Duration(rand.Intn(int(MaxElectionTimeout-MinElectionTimeout)))):
				rf.state = Candidate
			}
		case Candidate:
			rf.startElection()

			select {
			case <-time.After(MinElectionTimeout + time.Duration(rand.Intn(int(MaxElectionTimeout-MinElectionTimeout)))):
			case <-rf.newLeaderCh:
				rf.state = Leader
				rf.nextIndex = make([]int, len(rf.peers))
				rf.matchIndex = make([]int, len(rf.peers))
				for i := range rf.peers {
					rf.nextIndex[i] = rf.getLastLogIndex() + 1
					rf.matchIndex[i] = 0
				}
			case <-rf.heartbeat:
				rf.state = Follower
			}
		}
		// pause for a random amount of time between 50 and 350
		// milliseconds.
		ms := 50 + (rand.Int63() % 200)
		time.Sleep(time.Duration(ms) * time.Millisecond)
	}
}

func (rf *Raft) Start(command interface{}) (int, int, bool) {
	rf.mu.Lock()
	defer rf.mu.Unlock()
	if rf.state != Leader {
		return -1, -1, false
	}
	term := rf.currentTerm
	rf.log = append(rf.log, LogEntry{term, command})
	rf.persist()
	return rf.getLastLogIndex(), term, true
}

func Make(peers []*labrpc.ClientEnd, me int,
	persister *Persister, applyCh chan ApplyMsg) *Raft {
	rf := &Raft{}
	rf.peers = peers
	rf.persister = persister
	rf.me = me
	rf.logger = logger.NewLogger(me+1, true, fmt.Sprintf("raft-%d", me), constants.RaftLoggingMap)
	// Your initialization code here (2A, 2B, 2C).
	rf.currentTerm = 0
	rf.totalVotes = 0
	rf.votedFor = -1
	rf.commitIndex = 0
	rf.lastApplied = 0
	rf.state = Follower
	rf.applyCh = applyCh
	rf.newLeaderCh = make(chan bool)
	rf.voted = make(chan bool)
	rf.heartbeat = make(chan bool)

	rf.log = append(rf.log, LogEntry{Term: 0})

	// Initialize from state persisted before a crash.
	rf.readPersist(persister.ReadRaftState())
	rf.logger.Log(constants.LogRaftStart, "Raft server started")

	go rf.ticker()
	return rf
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
