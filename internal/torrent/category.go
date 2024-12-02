package torrent

//go:generate go run golang.org/x/tools/cmd/stringer -type Category
type Category int

const (
	MovieSingle Category = iota
	TvSingle
	TvSeason
	Manual
	Ignore
	Seed
)

var AllCategories []Category

func init() {
	// this only works because we're using iota, starting from zero
	AllCategories = make([]Category, len(_Category_index))
	for i := range _Category_index {
		AllCategories[i] = Category(i)
	}
}
