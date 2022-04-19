package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"

	"github.com/PuerkitoBio/goquery"
	"github.com/jmwoliver/VGDownloader/reps"
	"github.com/manifoldco/promptui"
)

func getAlbumURL() (reps.AlbumList, error) {

	title, err := getTitle()
	if err != nil {
		return reps.AlbumList{}, err
	}

	album, err := getAlbum(title)

	// Make the directory to put all the songs into
	dir := fmt.Sprintf("./%s/%s", reps.OutputDir, album.Title)
	err = os.MkdirAll(dir, 0777)
	if err != nil {
		return reps.AlbumList{}, err
	}

	return album, nil
}

func getAlbum(title string) (reps.AlbumList, error) {
	baseSearchURL := "https://downloads.khinsider.com/search?search="
	searchURL := baseSearchURL + title

	doc, err := getDocument(searchURL)
	if err != nil {
		return reps.AlbumList{}, err
	}

	albumList := getAlbumList(doc)

	album, err := selectAlbum(albumList)
	if err != nil {
		return reps.AlbumList{}, err
	}
	return album, nil

}

func selectAlbum(albumList []reps.AlbumList) (reps.AlbumList, error) {
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
		return reps.AlbumList{}, err
	}

	return albumList[index], nil
}

func getAlbumList(doc *goquery.Document) []reps.AlbumList {
	var albumList []reps.AlbumList
	doc.Find("#EchoTopic > p > a").Each(func(i int, s *goquery.Selection) {
		link, exist := s.Attr("href")
		if exist {
			al := reps.AlbumList{Title: s.Text(), Link: link}
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
	URL := reps.BaseURL + album.Link
	doc, err := getDocument(URL)
	if err != nil {
		log.Fatalf("%v\n", err)
	}

	// Use the HTML to get the file links and download them concurrently
	err = downloadFromDocument(doc, album.Title)
	if err != nil {
		log.Fatalf("%v\n", err)
	}
	fmt.Printf("\nDownload Complete!\n")

	downloadDir := fmt.Sprintf("%s/%s/%s", dir, reps.OutputDir, album.Title)
	fmt.Printf("Saved to %s\n", downloadDir)
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
	total := 0
	go getSongLinks(doc, links, &total)

	// Concurrently use the links from the buffered channel to download the songs
	err := downloadSongs(links, albumName, &total)
	if err != nil {
		return err
	}
	return nil
}

func getSongLinks(doc *goquery.Document, links chan<- string, total *int) {
	doc.Find(".playlistDownloadSong > a").Each(func(i int, s *goquery.Selection) {
		link, exist := s.Attr("href")
		if exist {
			links <- fmt.Sprintf("%s%s", reps.BaseURL, link)
			*total += 1
		}
	})
	close(links)
}

func downloadSongs(links <-chan string, albumName string, total *int) error {
	s := reps.NewSpinner("Completed...", total)
	wg := new(sync.WaitGroup)
	completed := 0
	go s.Loading(&completed, total)
	for link := range links {
		fileName := fmt.Sprintf("%v.mp3", completed)
		wg.Add(1)
		go downloadSong(wg, link, fileName, albumName, &completed)
	}
	wg.Wait()
	s.Finished()
	return nil
}

func downloadSong(wg *sync.WaitGroup, link string, fileName string, albumName string, completed *int) error {
	doc, err := getDocument(link)
	if err != nil {
		return err
	}
	// TODO add a --flac flag that can set the Eq(1) if it exists, if not default to 0
	encodedUrl, exist := doc.Find("#EchoTopic > p > a[href*='vgmsite']").Eq(0).Attr("href")
	if exist {
		err = downloadFile(encodedUrl, albumName)
		if err != nil {
			log.Fatalf("%v\n", err)
		}
	}
	wg.Done()
	*completed += 1
	return nil
}

func downloadFile(encodedUrl string, albumName string) error {
	resp, err := http.Get(encodedUrl)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	decodedUrl, err := url.QueryUnescape(encodedUrl)
	if err != nil {
		log.Fatal(err)
		return err
	}
	split := strings.Split(decodedUrl, "/")
	fileName := split[len(split)-1]

	filePath := fmt.Sprintf("./%s/%s/%s", reps.OutputDir, albumName, fileName)
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
