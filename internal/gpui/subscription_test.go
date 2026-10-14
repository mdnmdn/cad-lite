package gpui_test

import (
	"testing"

	"cad-lite/internal/gpui"
)

func TestSubscriptionCloseRunsOnce(t *testing.T) {
	calls := 0
	sub := gpui.NewSubscription(func() { calls++ })
	if sub.IsClosed() {
		t.Error("fresh Subscription should not be closed")
	}
	sub.Close()
	sub.Close()
	sub.Close()
	if calls != 1 {
		t.Errorf("close func ran %d times, want 1", calls)
	}
	if !sub.IsClosed() {
		t.Error("Subscription should report closed after Close")
	}
}

func TestSubscriptionCloseNilReceiver(t *testing.T) {
	var sub *gpui.Subscription
	sub.Close() // must not panic
	if !sub.IsClosed() {
		t.Error("nil Subscription should report closed")
	}
}

func TestSubscriptionNilCloseFunc(t *testing.T) {
	sub := gpui.NewSubscription(nil)
	sub.Close()
	sub.Close()
	if !sub.IsClosed() {
		t.Error("Subscription with nil close func should report closed")
	}
}

func TestSubscriberSetEmitCallsAll(t *testing.T) {
	set := gpui.NewSubscriberSet[string, func(int)]()
	var got []int
	set.Insert("k", func(v int) { got = append(got, v) })
	set.Insert("k", func(v int) { got = append(got, v+1) })
	set.Emit("k", func(cb func(int)) { cb(10) })
	if len(got) != 2 || got[0] != 10 || got[1] != 11 {
		t.Errorf("Emit called callbacks with %v, want [10 11]", got)
	}
	if set.Len() != 2 {
		t.Errorf("Len = %d, want 2", set.Len())
	}
}

func TestSubscriberSetCloseOne(t *testing.T) {
	set := gpui.NewSubscriberSet[string, func(int)]()
	var got []int
	set.Insert("k", func(v int) { got = append(got, v) })
	sub := set.Insert("k", func(v int) { got = append(got, v*2) })
	sub.Close()
	set.Emit("k", func(cb func(int)) { cb(1) })
	if len(got) != 1 || got[0] != 1 {
		t.Errorf("Emit after Close called %v, want [1]", got)
	}
	if set.Len() != 1 {
		t.Errorf("Len after Close = %d, want 1", set.Len())
	}
}

func TestSubscriberSetRemove(t *testing.T) {
	set := gpui.NewSubscriberSet[string, func(int)]()
	var got []int
	set.Insert("a", func(v int) { got = append(got, 1) })
	set.Insert("a", func(v int) { got = append(got, 2) })
	set.Insert("b", func(v int) { got = append(got, 3) })
	set.Remove("a")
	if set.Len() != 1 {
		t.Errorf("Len after Remove = %d, want 1", set.Len())
	}
	set.Emit("a", func(cb func(int)) { cb(0) })
	set.Emit("b", func(cb func(int)) { cb(0) })
	if len(got) != 1 || got[0] != 3 {
		t.Errorf("Emit after Remove called %v, want [3]", got)
	}
}

func TestSubscriberSetReentrantInsert(t *testing.T) {
	set := gpui.NewSubscriberSet[string, func()]()
	newCalls := 0
	inserted := false
	set.Insert("k", func() {
		if !inserted {
			inserted = true
			set.Insert("k", func() { newCalls++ })
		}
	})
	set.Emit("k", func(cb func()) { cb() })
	if newCalls != 0 {
		t.Errorf("callback inserted during Emit was invoked in the same Emit (%d calls)", newCalls)
	}
	set.Emit("k", func(cb func()) { cb() })
	if newCalls != 1 {
		t.Errorf("callback inserted during Emit was not invoked on the next Emit (%d calls)", newCalls)
	}
}

func TestSubscriberSetSelfCloseDuringEmit(t *testing.T) {
	set := gpui.NewSubscriberSet[string, func()]()
	calls := 0
	var sub *gpui.Subscription
	sub = set.Insert("k", func() {
		calls++
		sub.Close()
	})
	set.Insert("k", func() { calls += 100 })
	set.Emit("k", func(cb func()) { cb() })
	if calls != 101 {
		t.Errorf("first Emit calls = %d, want 101", calls)
	}
	set.Emit("k", func(cb func()) { cb() })
	if calls != 201 {
		t.Errorf("second Emit calls = %d, want 201 (self-closed callback must not run again)", calls)
	}
}

func TestSubscriberSetInsertOtherKeyDuringEmit(t *testing.T) {
	set := gpui.NewSubscriberSet[string, func()]()
	var order []string
	otherCalls := 0
	set.Insert("a", func() {
		order = append(order, "a1")
		set.Insert("b", func() { otherCalls++ })
	})
	set.Insert("a", func() { order = append(order, "a2") })
	set.Emit("a", func(cb func()) { cb() })
	if len(order) != 2 || order[0] != "a1" || order[1] != "a2" {
		t.Errorf("Emit(a) order = %v, want [a1 a2]", order)
	}
	if otherCalls != 0 {
		t.Errorf("callback on key b was invoked during Emit(a) (%d calls)", otherCalls)
	}
	set.Emit("b", func(cb func()) { cb() })
	if otherCalls != 1 {
		t.Errorf("callback on key b calls after Emit(b) = %d, want 1", otherCalls)
	}
}
