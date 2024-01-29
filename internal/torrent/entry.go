package torrent

type Entry struct {
	//copy --OutputPath=M:/ -N=The Seven Principles for Making - John Gottman.epub -L=Manual -F=D:\\Torrents\\Data\\The Seven Principles for Making - John Gottman.epub -C=1 -Z=4973007 -T=https://flacsfor.me/14469b453e4aa43266e03980013dd820/announce -I=6c9b2e9ea8b2857cd58870db45b26c9205d68a82 -D=D:\\Torrents\\Data","timestamp":"2023-01-09T08:31:38.365Z"}
	OutputPath    string
	Name          string
	Category      Category
	ContentPath   string
	NumberOfFiles int
	Size          int
	Tracker       string
	Hash          string
	SavePath      string
}
