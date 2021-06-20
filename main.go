package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/PuerkitoBio/goquery"
	"github.com/dhowden/tag"
	"github.com/manifoldco/promptui"
)

const (
	baseFilePath = "./output"
)

func getAlbumURL() (AlbumList, error) {

	title, err := getTitle()
	if err != nil {
		return AlbumList{}, err
	}

	album, err := getAlbum(title)

	// Make the directory to put all the songs into
	dir := fmt.Sprintf("%s/%s", baseFilePath, album.Title)
	err = os.MkdirAll(dir, 0777)
	if err != nil {
		return AlbumList{}, err
	}

	return album, nil
}

func getAlbum(title string) (AlbumList, error) {
	baseSearchURL := "https://downloads.khinsider.com/search?search="
	searchURL := baseSearchURL + title

	doc, err := getDocument(searchURL)
	if err != nil {
		return AlbumList{}, err
	}

	albumList := getAlbumList(doc)

	album, err := selectAlbum(albumList)
	if err != nil {
		return AlbumList{}, err
	}
	return album, nil

}

func selectAlbum(albumList []AlbumList) (AlbumList, error) {
	templates := &promptui.SelectTemplates{
		Active:   "⇀ {{ .Title | cyan }}",
		Inactive: "  {{ .Title | red }}",
		Selected: "{{ .Title | cyan }}"}
	prompt := promptui.Select{
		Label:     "Select Album",
		Items:     albumList,
		Templates: templates,
		Size:      10,
	}

	index, _, err := prompt.Run()

	if err != nil {
		return AlbumList{}, err
	}

	return albumList[index], nil
}

type AlbumList struct {
	Title string
	Link  string
}

func getAlbumList(doc *goquery.Document) []AlbumList {
	var albumList []AlbumList
	doc.Find("#EchoTopic > p > a").Each(func(i int, s *goquery.Selection) {
		link, exist := s.Attr("href")
		if exist {
			al := AlbumList{Title: s.Text(), Link: link}
			albumList = append(albumList, al)
		}
	})

	return albumList
}

func getTitle() (string, error) {
	prompt := promptui.Prompt{
		Label: "Game Title",
	}

	title, err := prompt.Run()
	if err != nil {
		return "", err
	}

	// Replace spaces with pluses for the URL
	title = strings.ReplaceAll(title, " ", "+")

	return title, nil

}

func main() {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	album, err := getAlbumURL()
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	// Get the HTML file
	baseURL := "https://downloads.khinsider.com"
	URL := baseURL + album.Link
	doc, err := getDocument(URL)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	downloadDir := fmt.Sprintf("%s/%s/%s", dir, "output", album.Title)
	fmt.Printf("Saving to: %s\n", downloadDir)

	// Use the HTML to get the file links and download them concurrently
	err = downloadFromDocument(doc, album.Title)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	fmt.Printf("Download Complete!\n")
}

// TODO this function feels super slow right now, see if there is a better way to do this
func getDocument(url string) (*goquery.Document, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return nil, err
	}
	return doc, nil
}

func downloadFromDocument(doc *goquery.Document, albumName string) error {
	// Parse HTML to get the links for each song, saving them to a buffered channel
	links := make(chan string, 100)
	go getSongLinks(doc, links)

	// Concurrently use the links from the buffered channel to download the songs
	err := downloadSongs(links, albumName)
	if err != nil {
		return err
	}
	return nil

}

func getSongLinks(doc *goquery.Document, links chan<- string) {
	baseURL := "https://downloads.khinsider.com"
	doc.Find(".playlistDownloadSong > a").Each(func(i int, s *goquery.Selection) {
		link, exist := s.Attr("href")
		if exist {
			links <- fmt.Sprintf("%s%s", baseURL, link)
		}
	})
	close(links)
}

type Spinner struct {
	spinChars string
	message   string
	i         int
}

func NewSpinner(message string) *Spinner {
	return &Spinner{spinChars: `|/-\`, message: message}
}

func (s *Spinner) Tick() {
	fmt.Printf("%s %c \r", s.message, s.spinChars[s.i])
	s.i = (s.i + 1) % len(s.spinChars)
	time.Sleep(100 * time.Millisecond)
}

// Loading screen
func Loading() {
	s := NewSpinner("Downloading...")
	for {
		s.Tick()
	}
}

func downloadSongs(links <-chan string, albumName string) error {

	wg := new(sync.WaitGroup)
	count := 0
	go Loading()
	for link := range links {
		fileName := fmt.Sprintf("%v.mp3", count)
		wg.Add(1)
		go downloadSong(wg, link, fileName, albumName)
		count += 1
	}
	wg.Wait()
	return nil
}

func downloadSong(wg *sync.WaitGroup, link string, fileName string, albumName string) error {
	doc, err := getDocument(link)
	if err != nil {
		return err
	}
	// TODO add a --flac flag that can set the Eq(1) if it exists, if not default to 0
	download, exist := doc.Find("#EchoTopic > p > a[href*='vgmsite']").Eq(0).Attr("href")
	if exist {
		// fmt.Printf("Download Link: %s\n", download)
		err = downloadFile(download, fileName, albumName)
		if err != nil {
			log.Fatalf("%v\n", err)
		}

		// TODO do this with channels since it's annoying and slow to have to reopen the file
		err = renameFile(fileName, albumName)
		if err != nil {
			log.Fatalf("%v\n", err)
		}
	}
	wg.Done()
	return nil
}

func renameFile(fileName string, albumName string) error {
	filePath := fmt.Sprintf("%s/%s/%s", baseFilePath, albumName, fileName)
	f, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer f.Close()
	err = os.Chmod(filePath, 0777)
	if err != nil {
		return err
	}

	m, err := tag.ReadFrom(f)
	if err != nil {
		log.Fatal(err)
	}

	newName := fmt.Sprintf("%s.mp3", m.Title())
	newFilePath := fmt.Sprintf("%s/%s/%s", baseFilePath, albumName, newName)
	err = os.Rename(filePath, newFilePath)
	if err != nil {
		return err
	}

	return nil
}

func downloadFile(url string, fileName string, albumName string) error {
	resp, err := http.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	filePath := fmt.Sprintf("%s/%s/%s", baseFilePath, albumName, fileName)
	f, err := os.Create(filePath)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	if err != nil {
		return err
	}
	return nil
}
