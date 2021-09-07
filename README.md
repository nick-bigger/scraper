# scraper

scraper is a multi-threaded web crawler for gathering phone numbers or URLs. This is a rewrite/repurpose of [Kuba Gretzky](https://twitter.com/mrgretzky)'s [dcrawl](https://github.com/kgretzky/dcrawl) to additionally scrape phone numbers.

## How it works?

scraper reads in a file of URLs and scrapes the site's body for either phone numbers or URLs. For phone numbers, it uses a simple regex. For URLs, it scans each site for `<a href=...>` links in the site's body.

* Crawls only sites that return *text/html* Content-Type in HEAD response.
* Retrieves site body of maximum 1MB size.
* Does not save inaccessible domains.
* Ignores duplicate domains.

## How to run?

```zsh
go build
./scraper -in input.csv -out output.csv -urls
```

## Usage

```
   ____________  ___   ___  _______ 
  / __/ ___/ _ \/ _ | / _ \/ __/ _ \
 _\ \/ /__/ , _/ __ |/ ___/ _// , _/
/___/\___/_/|_/_/ |_/_/  /___/_/|_| 
                              v.1.0

usage: ./scraper -in INPUT_FILE -out OUTPUT_FILE -urls|-phone_numbers

  -in string
        input file to read URLs from
  -out string
        output file to save scraped data to
  -urls bool
        tells scraper to scrape for URLs
  -phone_numbers bool
        tells scraper to scrape for phone numbers
  -t int
        number of concurrent threads (default 8)
  -v bool
        verbose (default false)
```

## License

dcrawl was made by [Kuba Gretzky](https://twitter.com/mrgretzky) from [breakdev.org](https://breakdev.org) and released under the MIT license. This rewrite/repurpose was made by Nick bigger and maintains the same MIT license.
