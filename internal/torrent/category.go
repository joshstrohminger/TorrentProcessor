package torrent

//go:generate go run golang.org/x/tools/cmd/stringer -type Category
type Category int

const (
	MovieSingle Category = iota
	TvSingle
	TvSeason
	Ignore
)

var AllCategories = []Category{MovieSingle, TvSingle, TvSeason, Ignore}
