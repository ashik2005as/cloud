package bid

func ValidBid(amount, currentHighest float64) bool {
	return amount > 0 && amount > currentHighest
}
