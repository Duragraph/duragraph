package worker

import "testing"

func TestAckDecision(t *testing.T) {
	const max = 5
	cases := []struct {
		name         string
		acked        bool
		epoch        int
		numDelivered int
		want         decision
	}{
		{"success acks", true, 1, 3, decisionAck},
		{"transient below max naks", false, 1, 4, decisionNak},
		{"transient at max with lease escalates", false, 1, 5, decisionEscalate},
		{"transient past max with lease escalates", false, 2, 6, decisionEscalate},
		{"transient at max without lease just acks", false, 0, 5, decisionAck},
		{"acked wins even at max", true, 0, 5, decisionAck},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ackDecision(c.acked, c.epoch, c.numDelivered, max); got != c.want {
				t.Errorf("ackDecision(%v,%d,%d,%d) = %v, want %v", c.acked, c.epoch, c.numDelivered, max, got, c.want)
			}
		})
	}
}
