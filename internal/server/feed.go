package server

import (
	"encoding/xml"
	"net/http"
	"strings"
	"time"

	"github.com/xiaotuantuan5/blog/internal/core"
)

// baseURL is the configured site URL, or one derived from the request.
func (s *Server) baseURL(r *http.Request, st core.Settings) string {
	if st.SiteURL != "" {
		return st.SiteURL
	}
	scheme := "http"
	if s.isHTTPS(r) {
		scheme = "https"
	}
	host := r.Host
	if s.cfg.TrustProxy {
		if h := r.Header.Get("X-Forwarded-Host"); h != "" {
			host = h
		}
	}
	return scheme + "://" + host
}

// absolutize turns root-relative links in note HTML into absolute ones,
// as feed readers resolve URLs inconsistently.
func absolutize(htmlStr, base string) string {
	r := strings.NewReplacer(`src="/`, `src="`+base+`/`, `href="/`, `href="`+base+`/`)
	return r.Replace(htmlStr)
}

type rss struct {
	XMLName xml.Name   `xml:"rss"`
	Version string     `xml:"version,attr"`
	Content string     `xml:"xmlns:content,attr"`
	Atom    string     `xml:"xmlns:atom,attr"`
	Channel rssChannel `xml:"channel"`
}

type rssChannel struct {
	Title       string    `xml:"title"`
	Link        string    `xml:"link"`
	Description string    `xml:"description"`
	Language    string    `xml:"language"`
	AtomLink    atomLink  `xml:"atom:link"`
	LastBuild   string    `xml:"lastBuildDate,omitempty"`
	Items       []rssItem `xml:"item"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr"`
	Type string `xml:"type,attr"`
}

type rssItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	GUID        string `xml:"guid"`
	PubDate     string `xml:"pubDate"`
	Description string `xml:"description"`
	Content     struct {
		Text string `xml:",cdata"`
	} `xml:"content:encoded"`
}

func (s *Server) handleRSS(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := s.app.Feed(r.Context(), 20, true)
	if err != nil {
		writeError(w, r, err)
		return
	}
	base := s.baseURL(r, st)
	feed := rss{
		Version: "2.0",
		Content: "http://purl.org/rss/1.0/modules/content/",
		Atom:    "http://www.w3.org/2005/Atom",
		Channel: rssChannel{
			Title:       st.SiteTitle,
			Link:        base + "/",
			Description: st.SiteDescription,
			Language:    "zh-CN",
			AtomLink:    atomLink{Href: base + "/rss.xml", Rel: "self", Type: "application/rss+xml"},
		},
	}
	if len(items) > 0 {
		feed.Channel.LastBuild = items[0].PublishedAt.Format(time.RFC1123Z)
	}
	for _, it := range items {
		link := base + "/n/" + it.Slug
		item := rssItem{
			Title:       it.Title,
			Link:        link,
			GUID:        link,
			PubDate:     it.PublishedAt.Format(time.RFC1123Z),
			Description: it.Summary,
		}
		// CDATA can't contain "]]>"; split it across sections.
		item.Content.Text = strings.ReplaceAll(absolutize(it.ContentHTML, base), "]]>", "]]]]><![CDATA[>")
		feed.Channel.Items = append(feed.Channel.Items, item)
	}
	w.Header().Set("Content-Type", "application/rss+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(feed)
}

type urlset struct {
	XMLName xml.Name     `xml:"urlset"`
	NS      string       `xml:"xmlns,attr"`
	URLs    []sitemapURL `xml:"url"`
}

type sitemapURL struct {
	Loc     string `xml:"loc"`
	LastMod string `xml:"lastmod,omitempty"`
}

func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	items, err := s.app.Feed(r.Context(), 0, false)
	if err != nil {
		writeError(w, r, err)
		return
	}
	base := s.baseURL(r, st)
	set := urlset{NS: "http://www.sitemaps.org/schemas/sitemap/0.9"}
	for _, p := range []string{"/", "/archive", "/tags", "/about"} {
		set.URLs = append(set.URLs, sitemapURL{Loc: base + p})
	}
	for _, it := range items {
		set.URLs = append(set.URLs, sitemapURL{Loc: base + "/n/" + it.Slug, LastMod: it.UpdatedAt.Format("2006-01-02")})
	}
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=600")
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(set)
}

func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	st, err := s.app.GetSettings(r.Context())
	if err != nil {
		writeError(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write([]byte("User-agent: *\nDisallow: /app\nDisallow: /login\nDisallow: /api/\n\nSitemap: " +
		s.baseURL(r, st) + "/sitemap.xml\n"))
}
