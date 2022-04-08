package paxosrsm

import (
	"time"

	"umich.edu/eecs491/proj5/paxos"
)

//
// additions to PaxosRSM state
//
type PaxosRSMImpl struct {
}

//
// initialize rsm.impl.*
//
func (rsm *PaxosRSM) InitRSMImpl() {
}

//
// application invokes AddOp to submit a new operation to the replicated log
// AddOp returns only once value v has been decided for some Paxos instance
//
func (rsm *PaxosRSM) AddOp(v interface{}) {
	seq := rsm.px.GetDoneValue() + 1
	for {
		to := 10 * time.Millisecond
		//could be more efficient if check whether px is dead here
		//seq may < Max() because holes can exist in Log
		if state, _ := rsm.px.Status(seq); state == paxos.Pending {
			//if seq-th local slot hasn't been filled
			//the application for seq-th slot will defintely fail if seq <= Max(), but it can make up holes in Log
			rsm.px.Start(seq, v)
		}
		for {
			state, operation := rsm.px.Status(seq)
			if state == paxos.Decided && rsm.equals(operation, v) {
				//operation updates successfully, now ApplyOp
				rsm.applyOp(operation)
				//forget opeartions which have been executed
				rsm.px.Done(seq)
				//Use the deleting function in Min() to remove all unnecessary states in px.Log
				rsm.px.Min()
				return
			} else if state == paxos.Decided && !rsm.equals(operation, v) {
				//seq-th slot has been occupied by another operation
				//increase seq by 1, and continue to apply for the next slot
				rsm.applyOp(operation)
				rsm.px.Done(seq)
				seq += 1
				break
			}
			//if state == Pending, keep waiting until this slot has been decided
			//state != Forgotten because seq > Done
			time.Sleep(to)
			if to < 10*time.Second {
				to *= 2
			} else {
				//server has already waited for too much time. AddOp fails
				return
			}
		}
	}
}
