// This is a rewrite/repurpose of https://github.com/kgretzky/dcrawl.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

const Version = "1.0"
const BodyLimit = 1024 * 1024
const UserAgent = "scraper/1.0"
const RequestDelay = 3

// Scraper Regexes
const PhoneRegex = `\(\d{3}\) \d{3}-\d{4}`
const AnchorRegex = `<a\s+(?:[^>]*?\s+)?href=["\']([^"\']*)`
const URLRegex = `^(?:ftp|http|https):\/\/(?:[\w\.\-\+]+:{0,1}[\w\.\-\+]*@)?(?:[a-z0-9\-\.]+)(?::[0-9]+)?(?:\/|\/(?:[\w#!:\.\?\+=&amp;%@!\-\/\(\)]+)|\?(?:[\w#!:\.\?\+=&amp;%@!\-\/\(\)]+))?$`

// Blacklists
var phoneBlacklist = []string{
	"(000) 000-0000",
	"(111) 111-1111",
	"(222) 222-2222",
	"(333) 333-3333",
	"(444) 444-4444",
	"(555) 555-5555",
	"(666) 666-6666",
	"(777) 777-7777",
	"(888) 888-8888",
	"(999) 999-9999",
}
var urlBlacklist = []string{
	"google.com", ".google.", "facebook.com", "twitter.com", ".gov", "youtube.com", "wikipedia.org", "wikisource.org", "wikibooks.org", "deviantart.com",
	"wiktionary.org", "wikiquote.org", "wikiversity.org", "wikia.com", "deviantart.com", "blogspot.", "wordpress.com", "tumblr.com", "about.com", "instagram.com", "wp-admin", "blog",
}

var visited = NewSet()

var http_client *http.Client

var (
	input_file           = flag.String("in", "", "input file to read URLs from")
	output_file          = flag.String("out", "", "output file to save scraped data to")
	max_threads          = flag.Int("t", 8, "number of concurrent threads (default 8)")
	verbose              = flag.Bool("v", false, "verbose (default false)")
	scrape_urls          = flag.Bool("urls", false, "whether to scrape URLs (default false)")
	scrape_phone_numbers = flag.Bool("phone_numbers", false, "whether to scrape phone numbers (default true)")
)

// A URL and the data we scraped from it.
type ScrapedUrl struct {
	url           string
	phone_numbers *Set
	urls          *Set
}

// stringInArray checks to see if s is in sa.
func stringInArray(s string, sa []string) bool {
	for _, x := range sa {
		if x == s {
			return true
		}
	}
	return false
}

// getHtml attempts to download the HTML from url.
func getHtml(url string) ([]byte, error) {
	req, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("User-Agent", UserAgent)

	resp, err := http_client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP response %d for %s", resp.StatusCode, url)
	}
	if _, ct_ok := resp.Header["Content-Type"]; ct_ok {
		ctypes := strings.Split(resp.Header["Content-Type"][0], ";")
		if !stringInArray("text/html", ctypes) {
			return nil, fmt.Errorf("URL is not 'text/html' for %s", url)
		}
	}

	req.Method = "GET"
	resp, err = http_client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Limit response reading to 1MB.
	b, err := ioutil.ReadAll(io.LimitReader(resp.Body, BodyLimit))
	if err != nil {
		return nil, err
	}

	return b, nil
}

// findAllUrls finds all URLs in html. If a relative URL is found, attempts
// to convert it to an absolute URL using domain.
func findAllUrls(domain string, html []byte) *Set {
	// Find all anchor tags.
	anchor_regex, _ := regexp.Compile(AnchorRegex)
	anchor_matches := anchor_regex.FindAllSubmatch(html, -1)

	urls := NewSet()

	// Find all valid URLs from the anchor tags' hrefs.
	url_regex, _ := regexp.Compile(URLRegex)
	for _, anchor_match := range anchor_matches {
		href := anchor_match[1]

		if url_regex.Match(href) {
			url := string(href)
			if !isBlacklisted(url, urlBlacklist) {
				urls.Add(url)
			}
		} else if len(anchor_match) > 0 && len(href) > 0 && href[0] == '/' {
			// Didn't match URL regex the first time and first character is a slash
			// - this is a relative URL. Add base domain details and try again.
			base_domain, err := url.Parse(domain)
			if err == nil {
				ur := base_domain.Scheme + "://" + base_domain.Host + string(href)
				if url_regex.MatchString(ur) {
					urls.Add(ur)
				}
			}
		}
	}

	return urls
}

// findAllPhoneNumbers scrapes all phone numbers it can find from html and
// returns them.
func findAllPhoneNumbers(html []byte) *Set {
	re, _ := regexp.Compile(PhoneRegex)
	phone_number_matches := re.FindAllSubmatch(html, -1)
	phone_numbers := NewSet()

	for _, phone_number_match := range phone_number_matches {
		phone_number := string(phone_number_match[0])
		if !isBlacklisted(phone_number, phoneBlacklist) {
			phone_numbers.Add(string(phone_number))
		}
	}
	return phone_numbers
}

// processUrls ranges through url_queue while it's open, downloading the
// website code from the URL, scrapes the HTML, then creates and sends
// a ScrapedUrl entry to scraped_urls.
func processUrls(url_queue <-chan string, scraped_urls chan<- ScrapedUrl) {
	for unscraped_url := range url_queue {
		if visited.Contains(unscraped_url) {
			continue
		} else {
			visited.Add(unscraped_url)
		}

		if *verbose {
			fmt.Printf("[->] %s\n", unscraped_url)
		}

		// Download the HTML from URL, continue if there's an error.
		html, err := getHtml(unscraped_url)
		if err != nil {
			if *verbose {
				fmt.Printf("[x] failed: %s\n", err)
			}
			continue
		}

		// NOTE: If adding something new to scrape for (like emails), add a
		// func call here and to the output below, as well as to the ScrapedUrl
		// struct.
		phone_numbers := NewSet()
		urls := NewSet()

		if *scrape_phone_numbers {
			phone_numbers = findAllPhoneNumbers(html)
		} else if *scrape_urls {
			urls = findAllUrls(unscraped_url, html)
		}

		scraped_urls <- ScrapedUrl{unscraped_url, phone_numbers, urls}

		time.Sleep(RequestDelay * time.Second)
	}
}

// isBlacklisted checks if s contains any blacklisted strings.
func isBlacklisted(s string, blacklist []string) bool {
	for _, blacklist_entry := range blacklist {
		if strings.Contains(s, blacklist_entry) {
			return true
		}
	}
	return false
}

// createHTTPClient creates and returns a configured HTTP Client.
func createHttpClient() *http.Client {
	var transport = &http.Transport{
		Dial: (&net.Dialer{
			Timeout: 10 * time.Second,
		}).Dial,
		TLSHandshakeTimeout: 5 * time.Second,
		DisableKeepAlives:   true,
	}
	client := &http.Client{
		Timeout:   time.Second * 10,
		Transport: transport,
	}
	return client
}

// loadUrls scans through file and passes URLs into the url_queue channel.
// After running, both the channel and file are closed.
func loadUrls(file *os.File, url_queue chan<- string) {
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		input_url := scanner.Text()
		if _, err := url.Parse(input_url); err == nil {
			url_queue <- input_url
		}
	}
	close(url_queue)
	file.Close()
}

// banner prints out the banner header when running the script.
func banner() {
	fmt.Println(`   ____________  ___   ___  _______ `)
	fmt.Println(`  / __/ ___/ _ \/ _ | / _ \/ __/ _ \`)
	fmt.Println(` _\ \/ /__/ , _/ __ |/ ___/ _// , _/`)
	fmt.Println(`/___/\___/_/|_/_/ |_/_/  /___/_/|_| `)
	fmt.Println(`                              v.` + Version)
	fmt.Println("")
}

// usage prints out useful information about running this script.
func usage() {
	fmt.Printf("usage: ./scraper -in INPUT_FILE -out OUTPUT_FILE -urls|-phone_numbers\n\n")
}

func init() {
	http_client = createHttpClient()
}

func main() {
	banner()

	flag.Parse()

	if *input_file == "" || *output_file == "" {
		usage()
		return
	}

	if !*scrape_phone_numbers && !*scrape_urls {
		usage()
		return
	}

	fmt.Printf("[*] input file: %s\n", *input_file)
	fmt.Printf("[*] output file: %s\n", *output_file)
	fmt.Printf("[*] max threads: %d\n", *max_threads)
	if *scrape_phone_numbers {
		fmt.Printf("[*] scraping: phone_numbers\n")
	} else if *scrape_urls {
		fmt.Printf("[*] scraping: urls\n")
	}
	fmt.Printf("\n")

	// Open input file for reading.
	f_input, err := os.Open(*input_file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: can't open file '%s'", *input_file)
		return
	}

	// Open output file for writing to.
	f_output, err := os.OpenFile(*output_file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if os.IsNotExist(err) {
		f_output, err = os.Create(*output_file)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: can't open or create file '%s'", *output_file)
		return
	}
	defer f_output.Close()

	// url_queue provides input for scraper threads, out_parsed
	// handles output from them.
	var wg sync.WaitGroup
	url_queue := make(chan string)
	out_parsed := make(chan ScrapedUrl)

	// Create {max_threads} threads to process incoming URLs.
	for i := 0; i < *max_threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			processUrls(url_queue, out_parsed)
		}()
	}

	// This will input URLs as they are scanned in into our
	// running scraper threads.
	go loadUrls(f_input, url_queue)

	// Once all of our scraper threads have finished, close the output
	// so main can finish.
	go func() {
		wg.Wait()
		close(out_parsed)
	}()

	// Handle output URLs/scraped data and write to output file.
	w := bufio.NewWriter(f_output)
	for purl := range out_parsed {
		fmt.Printf("[%s] %s\n", "<-", purl.url)

		if *scrape_phone_numbers {
			fmt.Fprintf(w, "%s, %s\n", purl.url, purl.phone_numbers)
		} else if *scrape_urls {
			output := ""
			for url := range purl.urls.list {
				output += fmt.Sprintf("%s\n", url)
			}
			fmt.Fprintf(w, output)
		}

		w.Flush()
	}
}
