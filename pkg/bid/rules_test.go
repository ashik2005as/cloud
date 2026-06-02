package bid

import "testing"

func TestValidBid(t *testing.T) {
	if !ValidBid(120, 100) {
		t.Fatal("expected valid bid")
	}
	if ValidBid(100, 100) {
		t.Fatal("equal bid should be invalid")
	}
	if ValidBid(-1, 0) {
		t.Fatal("negative bid should be invalid")
	}
}
