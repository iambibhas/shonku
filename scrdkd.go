package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"github.com/gorilla/feeds"
	"github.com/russross/blackfriday"
	"html/template"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// All structures below are required for rendering different webpages.

type Configuration struct {
	Author         string
	Title          string
	URL            string
	Content_footer string
	Disqus         string
	Email          string
	Description    string
	Logo           string
	Links          []PageLink
}

type PageLink struct {
	Link string
	Text string
}

type ArchiveLink struct {
	Time_str string
	Url      string
	Text     string
}

type Article struct {
	Id   int
	Path string
	Hash string
}

type Post struct {
	Title   string
	Slug    string
	Body    template.HTML
	Date    time.Time
	Tags    map[string]string
	Changed bool
	Url     string
	Durl    template.JSStr
	Logo    string
	Links   []PageLink
	Disqus  string
}

type Catpage struct {
	Cats   map[string]string
	Logo   string
	Links  []PageLink
	Disqus bool
}

type Archivepage struct {
	Years  []string
	Logo   string
	Links  []PageLink
	Disqus bool
}

type Archivelist struct {
	Year    string
	ArLinks []ArchiveLink
	Logo    string
	Links   []PageLink
	Disqus  bool
}

type Indexposts struct {
	Posts     []Post
	NextF     bool
	PreviousF bool
	Next      int
	Previous  int
	NextLast  bool
	Logo      string
	Links     []PageLink
	Main      bool
	Disqus    bool
}

type Sitemap struct {
	Loc      string `xml:"loc"`
	Lastmod  string `xml:"lastmod"`
	Priority string `xml:"priority"`
}

type SiteDB map[string]Sitemap

type FileDB map[string]string

var current_time time.Time
var SDB SiteDB
var FDB FileDB
var conf Configuration
var POSTN int

type ByDate []Post

func (a ByDate) Len() int           { return len(a) }
func (a ByDate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByDate) Less(i, j int) bool { return a[i].Date.After(a[j].Date) }

type ByODate []Post

func (a ByODate) Len() int           { return len(a) }
func (a ByODate) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a ByODate) Less(i, j int) bool { return a[j].Date.After(a[i].Date) }

/*
Creates the empty build database.
*/
func createdb() {
	filename := ".scrdkd.json"
	m := make(map[string]string, 0)
	m["file"] = "filehash"
	if !exists(filename) {
		f, _ := os.Create(filename)
		enc := json.NewEncoder(f)
		enc.Encode(m)
		f.Close()

	}
}

/*
Reads the file database
*/
func get_fdb() FileDB {
	file, err := os.Open(".scrdkd.json")
	if err != nil {
		panic(err)
	}
	decoder := json.NewDecoder(file)
	//configuration := make(Configuration)
	var fdb FileDB
	err = decoder.Decode(&fdb)
	if err != nil {
		panic(err)
	}
	return fdb
}

/*
Finds all the files from our posts directory.
*/
func findfiles(dir string) []string {
	files, _ := ioutil.ReadDir(dir)
	names := make([]string, 0)
	for _, f := range files {
		names = append(names, dir+f.Name())
	}
	return names
}

/*
Creates the hash for each files.
*/
func create_hash(filename string) string {
	md, err := ioutil.ReadFile(filename)
	if err == nil {
		data := sha256.Sum256(md)
		data2 := data[:]
		s := base64.URLEncoding.EncodeToString(data2)
		return s
		//fmt.Println(s)
	}
	return ""
}

/*
Finds out if the file content changed from the last build.
*/
func changed_ornot(filename, hash string) bool {
	if val, ok := FDB[filename]; ok {
		if val == hash {
			return false
		}
	} else {
		FDB[filename] = hash
	}
	return true
}

/*
Returns the slug of the article
TODO: We need a replacement for
http://code.activestate.com/recipes/577257-slugify-make-a-string-usable-in-a-url-or-filename/
We need to improve this function much more.
*/
func get_slug(s string) string {
	// removing unwanted characters
	slug := regexp.MustCompile("[^a-zA-Z0-9-]").ReplaceAllString(s, "-")

	// collapsing dashes
	slug = regexp.MustCompile("-+").ReplaceAllString(slug, "-")

	// removing beginning and ending dashes
	slug = strings.Trim(slug, "-")

	return strings.ToLower(slug)
}

/*
Reads a post and gets all details from it.
*/
func read_post(filename string, conf Configuration) Post {
	var buffer bytes.Buffer
	var p Post
	flag := false
	f, err := os.Open(filename)
	if err != nil {
		fmt.Println(err)
		return p
	}
	defer f.Close()
	r := bufio.NewReader(f)
	titleline, err := r.ReadString('\n')
	dateline, err := r.ReadString('\n')
	line, err := r.ReadString('\n')
	tagline := line

	for err == nil {
		if line == "====\n" {
			flag = true
			line, err = r.ReadString('\n')
			continue
		}
		if flag {
			buffer.WriteString(line)
		}
		line, err = r.ReadString('\n')
	}

	if err == io.EOF {
		title := titleline[6:]
		title = strings.TrimSpace(title)
		date := dateline[5:]
		date = strings.TrimSpace(date)
		tagsnonstripped := strings.Split(tagline[5:], ",")
		tags := make(map[string]string, 0)
		for i := range tagsnonstripped {
			word := strings.TrimSpace(tagsnonstripped[i])
			tags[get_slug(word)] = word
		}

		p.Title = title
		slug := filepath.Base(filename)
		length := len(slug)
		p.Slug = slug[:length-3]
		body := blackfriday.MarkdownCommon(buffer.Bytes())
		p.Body = template.HTML(string(body))
		p.Date = get_time(date)
		p.Tags = tags
		p.Changed = false
		p.Url = fmt.Sprintf("%sposts/%s.html", conf.URL, p.Slug)
		p.Durl = template.JSStr(p.Url)
		p.Logo = conf.Logo
		p.Links = conf.Links
		p.Disqus = conf.Disqus

	}
	return p
}

func get_time(text string) time.Time {
	const longform = "2006-01-02 15:04:05.999999999 -0700 MST"
	//now := time.Now()
	//x := now.String()
	//fmt.Println(x)
	new_now, err := time.Parse(longform, text)
	if err != nil {
		fmt.Println(err)
	}
	return new_now

}

/*
Creates the atom and rss feeds.
*/
func build_feeds(ps []Post, conf Configuration, name string) {
	now := time.Now()
	feed := &feeds.Feed{
		Title:       conf.Title,
		Link:        &feeds.Link{Href: conf.URL},
		Description: conf.Description,
		Author:      &feeds.Author{conf.Author, conf.Email},
		Created:     now,
	}
	items := make([]*feeds.Item, 0)
	var item *feeds.Item
	for i := range ps {
		post := ps[i]
		if post.Changed {
			item = &feeds.Item{
				Title:       post.Title,
				Description: string(post.Body),
				Created:     post.Date,
				Updated:     now,
				Author:      &feeds.Author{conf.Author, conf.Email},
				Link:        &feeds.Link{Href: post.Url},
			}

		} else { // Post not changed, so keeping same old date.
			item = &feeds.Item{
				Title:       post.Title,
				Description: string(post.Body),
				Created:     post.Date,
				Updated:     post.Date,
				Author:      &feeds.Author{conf.Author, conf.Email},
				Link:        &feeds.Link{Href: post.Url},
			}
		}
		items = append(items, item)
	}

	feed.Items = items
	rss, err := feed.ToRss()
	atom, err := feed.ToAtom()
	if err != nil {
		fmt.Println(err)
	} else {
		if name == "cmain" {
			f, _ := os.Create("./output/rss.xml")
			defer f.Close()
			io.WriteString(f, rss)
			f2, _ := os.Create("./output/atom.xml")
			defer f2.Close()
			io.WriteString(f2, atom)
		} else {
			f, _ := os.Create("./output/categories/" + name + ".xml")
			defer f.Close()
			io.WriteString(f, rss)
		}
	}

}

/*
Builds a post based on the template
*/
func build_post(ps Post, ptype string) string {
	var doc bytes.Buffer
	var body, name string
	var err error
	var tml *template.Template
	if ptype == "post" {
		tml, err = template.ParseFiles("./templates/post.html", "./templates/base.html")
		name = "./output/posts/" + ps.Slug + ".html"
	} else {
		// This should read the pages template
		tml, err = template.ParseFiles("./templates/post.html", "./templates/base.html")
		name = "./output/pages/" + ps.Slug + ".html"
	}
	err = tml.ExecuteTemplate(&doc, "base", ps)
	if err != nil {
		fmt.Println(err)
	}
	body = doc.String()

	f, err := os.Create(name)
	defer f.Close()
	n, err := io.WriteString(f, body)

	if err != nil {
		fmt.Println(n, err)
	}

	return body
}

/*
Creates index pages.
*/
func build_index(pss []Post, index, pre, next int, indexname string) {
	fmt.Println(index, pre, next)
	var doc bytes.Buffer
	var body, name string
	var ips Indexposts
	ips.Posts = pss
	if pre != 0 {
		ips.PreviousF = true
		ips.Previous = pre
	} else {
		ips.PreviousF = false
	}
	if next > 0 {
		ips.NextF = true
		ips.Next = next
	} else if next == -1 {
		ips.NextF = false
	} else {
		ips.NextF = true
		ips.Next = next
	}
	if next == 0 {
		ips.NextLast = true
	}

	ips.Links = conf.Links
	ips.Logo = conf.Logo
	if indexname == "index" {
		ips.Main = true
	} else {
		ips.Main = false
	}
	ips.Disqus = false
	tml, err := template.ParseFiles("./templates/index.html", "./templates/base.html")
	if err != nil {
		fmt.Println(err)
	}
	err = tml.ExecuteTemplate(&doc, "base", ips)
	if err != nil {
		fmt.Println(err)
	}
	body = doc.String()
	if next == -1 {
		if indexname == "index" {
			name = fmt.Sprintf("./output/%s.html", indexname)
		} else {
			name = fmt.Sprintf("./output/categories/%s.html", indexname)
		}
	} else {
		if indexname == "index" {
			name = fmt.Sprintf("./output/%s-%d.html", indexname, index)
		} else {
			name = fmt.Sprintf("./output/categories/%s-%d.html", indexname, index)
		}
	}
	f, err := os.Create(name)
	defer f.Close()
	n, err := io.WriteString(f, body)

	if err != nil {
		fmt.Println(n, err)
	}
	// For Sitemap
	smap := Sitemap{Loc: conf.URL + name[9:], Lastmod: current_time.Format("2006-01-02"), Priority: "0.5"}
	SDB[smap.Loc] = smap
}

/*
Checks for path.
*/
func exists(path string) bool {
	_, err := os.Stat(path)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return false
}

/*
Creates the required directory structure.
*/
func create_dirs() {
	if !exists("./templates/") {
		os.Mkdir("./templates/", 0777)
	}
	if !exists("./output/") {
		os.Mkdir("./output/", 0777)
	}
	if !exists("./output/posts/") {
		os.Mkdir("./output/posts/", 0777)
	}
	if !exists("./output/pages/") {
		os.Mkdir("./output/pages/", 0777)
	}
	if !exists("./output/categories/") {
		os.MkdirAll("./output/categories/", 0777)
	}
	if !exists("./posts/") {
		os.Mkdir("./posts/", 0777)
	}
	if !exists("./pages/") {
		os.Mkdir("./pages/", 0777)
	}
	if !exists("./assets/css") {
		os.MkdirAll("./assets/css", 0777)
	}
	if !exists("./assets/css/images/ie6") {
		os.MkdirAll("./assets/css/images/ie6", 0777)
	}
	if !exists("./assets/js") {
		os.Mkdir("./assets/js", 0777)
	}
	if !exists("./assets/img") {
		os.Mkdir("./assets/img", 0777)
	}
	if !exists("./assets/img/icons") {
		os.Mkdir("./assets/img/icons", 0777)
	}
}

// Creates the static files for the theme.
func create_theme_files() {
	names := []string{
		"assets/js/imagesloaded.pkgd.min.js",
		"assets/js/slides.jquery.js",
		"assets/js/flowr.plugin.js",
		"assets/js/jquery-1.7.2.min.js",
		"assets/js/jquery.colorbox-min.js",
		"assets/js/bootstrap.js",
		"assets/js/slides.min.jquery.js",
		"assets/js/jquery-1.10.2.min.js",
		"assets/js/bootstrap.min.js",
		"assets/js/mathjax.js",

		"assets/img/icons",
		"assets/img/icons/twitter.png",
		"assets/img/icons/rss.png",
		"assets/img/icons/github.png",
		"assets/img/glyphicons-halflings-white.png",
		"assets/img/glyphicons-halflings.png",

		"assets/css/theme.css",
		"assets/css/code.css",
		"assets/css/bootstrap-responsive.css",
		"assets/css/slides.css",
		"assets/css/bootstrap.css",
		"assets/css/custom.css",
		"assets/css/bootstrap.min.css",
		"assets/css/rst.css",
		"assets/css/bootstrap-responsive.min.css",

		"assets/css/images/controls.png",
		"assets/css/images/loading_background.png",

		"assets/css/images/ie6/borderMiddleLeft.png",
		"assets/css/images/ie6/borderBottomRight.png",
		"assets/css/images/ie6/borderTopRight.png",
		"assets/css/images/ie6/borderMiddleRight.png",
		"assets/css/images/ie6/borderBottomLeft.png",
		"assets/css/images/ie6/borderBottomCenter.png",
		"assets/css/images/ie6/borderTopCenter.png",
		"assets/css/images/ie6/borderTopLeft.png",
		"assets/css/images/border.png",
		"assets/css/images/overlay.png",
		"assets/css/images/loading.gif",
		"assets/css/style.css",
		"assets/css/colorbox.css",
		"templates/index.html",
		"templates/category-index.html",
		"templates/post.html",
		"templates/base.html",
		"templates/archive.html",
		"templates/year.html"}
	for i := range names {
		name := names[i]
		data, _ := Asset(name)
		if !exists(name) {
			f, _ := os.Create(name)
			defer f.Close()
			io.WriteString(f, string(data))
		}
	}

}

/*
Creates new site
*/
func create_site() {

	create_dirs()
	create_theme_files()
}

/*
Reads and returns the configuration.
*/
func get_conf() Configuration {
	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	//configuration := make(Configuration)
	var configuration Configuration
	err := decoder.Decode(&configuration)
	if err != nil {
		fmt.Println(err)
	}
	return configuration
}

/*
Builds the categories pages and indexes
*/
func build_categories(cat Catpage) {
	var doc bytes.Buffer
	var body string
	tml, err := template.ParseFiles("./templates/category-index.html", "./templates/base.html")
	if err != nil {
		fmt.Println(err)
	}
	cat.Disqus = false
	err = tml.ExecuteTemplate(&doc, "base", cat)
	if err != nil {
		fmt.Println(err)
	}
	body = doc.String()
	name := "./output/categories/index.html"
	f, err := os.Create(name)
	defer f.Close()
	n, err := io.WriteString(f, body)

	if err != nil {
		fmt.Println(n, err)
	}
	// For Sitemap
	smap := Sitemap{Loc: conf.URL + "categories/index.html", Lastmod: current_time.Format("2006-01-02"), Priority: "0.5"}
	SDB[smap.Loc] = smap
}

func create_archive(years map[string][]Post) {
	yearslist := make([]string, 0)
	for k, _ := range years {
		yearslist = append(yearslist, k)
	}
	sort.Sort(sort.Reverse(sort.StringSlice(yearslist)))
	archive := Archivepage{Years: yearslist, Links: conf.Links, Logo: conf.Logo}

	//First create the archive index page.
	var doc bytes.Buffer
	var body string
	tml, err := template.ParseFiles("./templates/archive.html", "./templates/base.html")
	if err != nil {
		fmt.Println(err)
	}
	archive.Disqus = false
	err = tml.ExecuteTemplate(&doc, "base", archive)
	if err != nil {
		fmt.Println(err)
	}
	body = doc.String()
	name := "./output/archive.html"
	f, err := os.Create(name)

	n, err := io.WriteString(f, body)
	f.Close()
	if err != nil {
		fmt.Println(n, err)
	}
	// For Sitemap
	smap := Sitemap{Loc: conf.URL + "archive.html", Lastmod: current_time.Format("2006-01-02"), Priority: "0.5"}
	SDB[smap.Loc] = smap
	//Now create indivitual pages for each year.
	for k, v := range years {
		var doc bytes.Buffer
		ps := make([]ArchiveLink, 0)
		posts := v
		sort.Sort(ByDate(posts))
		for i := range posts {
			p := posts[i]
			ps = append(ps, ArchiveLink{Time_str: p.Date.Format("[2006-01-02 15:04:05]"),
				Url:  fmt.Sprintf("/posts/%s.html", p.Slug),
				Text: p.Title})
		}
		ar := Archivelist{Year: k, ArLinks: ps, Logo: conf.Logo, Links: conf.Links}
		ar.Disqus = false
		tml2, err := template.ParseFiles("./templates/year.html", "./templates/base.html")
		if err != nil {
			fmt.Println(err)
		}
		err = tml2.ExecuteTemplate(&doc, "base", ar)
		if err != nil {
			fmt.Println(err)
		}
		body := doc.String()
		name := fmt.Sprintf("./output/%s.html", k)
		f, err := os.Create(name)
		n, err := io.WriteString(f, body)
		f.Close()
		// For Sitemap
		smap := Sitemap{Loc: conf.URL + k + ".html", Lastmod: current_time.Format("2006-01-02"), Priority: "0.5"}
		SDB[smap.Loc] = smap
		if err != nil {
			fmt.Println(n, err)
		}

	}
}

/*
Creates the index pages as required.
*/
func create_index_files(ps []Post, indexname string) {
	var prev, next int
	index := 1
	num := 0
	length := len(ps)
	sort_index := make([]Post, 0)
	for i := range ps {
		sort_index = append(sort_index, ps[i])
		num = num + 1
		if num == POSTN {
			sort.Sort(ByDate(sort_index))
			if index == 1 {
				prev = 0
			} else {
				prev = index - 1
			}
			if (index*POSTN) < length && (length-index*POSTN) > POSTN {
				next = index + 1
			} else if (index * POSTN) == length {
				next = -1
			} else {
				next = 0
			}
			build_index(sort_index, index, prev, next, indexname)

			sort_index = make([]Post, 0)
			index = index + 1
			num = 0

		}
	}
	if len(sort_index) > 0 {
		sort.Sort(ByDate(sort_index))
		build_index(sort_index, 0, index-1, -1, indexname)

	}
}

/*
This rebuilds the whole site.
Any chnage to the configuration file will force this.
*/
func site_rebuild(rebuild, rebuild_index bool) {

	var indexlist []Post
	ps := make([]Post, 0)

	cat_needs_build := make(map[string]bool, 0)
	catslinks := make(map[string][]Post, 0)

	catnames := make(map[string]string, 0)
	pageyears := make(map[string][]Post, 0)
	names := findfiles("./posts/")
	for i := range names {
		hash := create_hash(names[i])
		post := read_post(names[i], conf)
		//Mark the date of the post.
		postdate := strconv.Itoa(post.Date.Year())
		pageyears[postdate] = append(pageyears[postdate], post)
		for k, v := range post.Tags {
			catnames[k] = v
			catslinks[k] = append(catslinks[k], post)
		}

		// For Sitemap
		smap := Sitemap{Loc: post.Url, Lastmod: post.Date.Format("2006-01-02"), Priority: "0.5"}

		if rebuild || changed_ornot(names[i], hash) {
			fmt.Println(names[i])
			build_post(post, "post")
			rebuild_index = true
			// Also mark that this post was changed on disk
			post.Changed = true
			smap.Lastmod = current_time.Format("2006-01-02")

			//Mark all categories need to be rebuild
			for i := range post.Tags {
				name := post.Tags[i]
				catslug := get_slug(name)
				cat_needs_build[catslug] = true
			}

		}
		ps = append(ps, post)
		SDB[post.Url] = smap
	}

	// Now let us build the static pages.
	names = findfiles("./pages/")
	for i := range names {
		hash := create_hash(names[i])
		post := read_post(names[i], conf)
		// For Sitemap
		smap := Sitemap{Loc: post.Url, Lastmod: post.Date.Format("2006-01-02"), Priority: "0.5"}

		if rebuild || changed_ornot(names[i], hash) {
			fmt.Println(names[i])
			build_post(post, "page")
			smap.Lastmod = current_time.Format("2006-01-02")
		}
		SDB[post.Url] = smap
	}

	cat := Catpage{Cats: catnames, Links: conf.Links, Logo: conf.Logo}
	build_categories(cat)

	//Now create index(s) for categories.
	for k, _ := range cat_needs_build {
		localps := catslinks[k]
		create_index_files(localps, k)
		//Now build the feeds as required.
		sort.Sort(ByDate(localps))
		if len(localps) >= 10 {
			indexlist = localps[:10]
		} else {
			indexlist = localps[:]
		}
		build_feeds(indexlist, conf, k)
	}

	// Now let us create the archive pages.
	create_archive(pageyears)

	sort.Sort(ByODate(ps))

	// If required then rebuild the primary index pages.
	if rebuild_index == true {
		create_index_files(ps, "index")
		// Time to check for any change in 10 posts at max and rebuild rss feed if required.

		sort.Sort(ByDate(ps))
		if len(ps) >= 10 {
			indexlist = ps[:10]
		} else {
			indexlist = ps[:]
		}
		build_feeds(indexlist, conf, "cmain")

		// We are using system installed rsync for this.
		curpath, _ := filepath.Abs(".")
		frompath := curpath + "/assets/"
		topath := curpath + "/output/assets/"
		cmd := exec.Command("rsync", "-avz", frompath, topath)
		var out bytes.Buffer
		cmd.Stdout = &out
		err := cmd.Run()
		if err != nil {
			fmt.Println(err)
		}
	}
}

/*
Saves the file build database
*/
func save_fdb() {
	f, _ := os.Create(".scrdkd.json")
	enc := json.NewEncoder(f)
	enc.Encode(FDB)
	f.Close()
}

func main() {

	POSTN = 10 // Magic number of posts in every index.

	new_site := flag.Bool("new_site", false, "Creates a new site in the current directory.")
	newpost := flag.Bool("new", false, "Creates a new post.")
	newpage := flag.Bool("new_page", false, "Creates a new page.")
	force := flag.Bool("force", false, "Force rebuilding of the whole site.")
	flag.Parse()

	if *new_site {
		create_site()
		os.Exit(0)
	}

	current_time = time.Now().Local()
	SDB = make(SiteDB, 0)
	conf = get_conf()

	// Get the build file database.
	createdb()
	FDB = get_fdb()
	defer save_fdb()
	defer create_sitemap()

	if *newpost {
		new_post()
		os.Exit(0)
	}

	if *newpage {
		new_page()
		os.Exit(0)
	}

	if *force {
		site_rebuild(true, true)
		create_sitemap()
		os.Exit(0)
	}

	fmt.Println(conf)

	rebuild_index := false
	site_rebuild(false, rebuild_index)

}

/*
To create the sitemap xml file
*/
func create_sitemap() {
	type Urlset struct {
		XMLName  xml.Name  `xml:"urlset"`
		Xmlns    string    `xml:"xmlns,attr"`
		Xmlnsxsi string    `xml:"xmlns:xsi,attr"`
		Xsi      string    `xml:"xsi:schemaLocation,attr"`
		Urls     []Sitemap `xml:"url"`
	}
	v := Urlset{Xmlns: "http://www.sitemaps.org/schemas/sitemap/0.9", Xmlnsxsi: "http://www.w3.org/2001/XMLSchema-instance",
		Xsi: "http://www.sitemaps.org/schemas/sitemap/0.9 http://www.sitemaps.org/schemas/sitemap/0.9/sitemap.xsd"}

	urls := make([]Sitemap, 0)
	for _, v := range SDB {
		urls = append(urls, v)
	}

	v.Urls = urls
	f, _ := os.Create("./output/sitemap.xml")
	defer f.Close()
	enc := xml.NewEncoder(f)
	enc.Indent("  ", "    ")
	if err := enc.Encode(v); err != nil {
		fmt.Println("error: %   v\n", err)
	}

}
