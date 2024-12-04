package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"github.com/go-rod/rod/lib/proto"
	"github.com/urfave/cli/v2"
)

type Course struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type Content struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	DisplayName string `json:"display_name"`
}

type CourseData struct {
	Course   Course    `json:"course"`
	Contents []Content `json:"contents"`
}

type VideoItem struct {
	Index       int    `json:"index"`
	DynamicPart string `json:"dynamic-part,omitempty"`
	Downloaded  bool   `json:"downloaded"`
	Name        string `json:"name"`
	Type        string `json:"type"` // "video" or "text"
}

type OutputData struct {
	Name      string      `json:"name"`
	ItemCount int         `json:"item-count"`
	Items     []VideoItem `json:"items"`
}

func main() {
	app := &cli.App{
		Name:  "ardan-labs-scraper",
		Usage: "Scrape Ardan Labs course videos and text content for downloading",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "cookie",
				Aliases:  []string{"c"},
				Usage:    "remember_user_token cookie value",
				Required: true,
			},
			&cli.StringFlag{
				Name:     "response",
				Aliases:  []string{"r"},
				Usage:    "Path to response.json file containing course metadata",
				Required: true,
			},
		},
		Action: run,
	}

	if err := app.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func sanitizeFilename(filename string) string {
	// Replace any chars that might be problematic in filenames
	invalid := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := filename
	for _, char := range invalid {
		result = strings.ReplaceAll(result, char, "_")
	}
	return result
}

func run(c *cli.Context) error {
	responseData, err := parseResponseFile(c.String("response"))
	if err != nil {
		return fmt.Errorf("failed to parse response file: %v", err)
	}

	// Create necessary directories
	if err := os.MkdirAll("jsons", 0755); err != nil {
		return fmt.Errorf("failed to create jsons directory: %v", err)
	}

	// Create course directory for HTML files
	courseDir := sanitizeFilename(responseData.Course.Name)
	if err := os.MkdirAll(courseDir, 0755); err != nil {
		return fmt.Errorf("failed to create course directory: %v", err)
	}

	browser := initBrowser()
	defer browser.MustClose()

	page := browser.MustPage()
	err = page.SetCookies([]*proto.NetworkCookieParam{
		{
			Name:     "remember_user_token",
			Value:    c.String("cookie"),
			Domain:   "courses.ardanlabs.com",
			Path:     "/",
			Secure:   true,
			HTTPOnly: true,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to set cookie: %v", err)
	}

	items := []VideoItem{}

	for idx, content := range responseData.Contents {
		isText := strings.Contains(strings.ToLower(content.DisplayName), "text")
		contentURL := fmt.Sprintf("https://courses.ardanlabs.com/courses/take/%s/%s/%s",
			responseData.Course.Slug,
			map[bool]string{true: "texts", false: "lessons"}[isText],
			content.Slug)

		fmt.Printf("Processing %s (%s) (%d/%d)\n",
			content.Name,
			map[bool]string{true: "Text", false: "Video"}[isText],
			idx+1,
			len(responseData.Contents))

		var item VideoItem
		var processErr error

		if isText {
			item, processErr = processHTMLContent(page, contentURL, idx, content, courseDir)
		} else {
			item, processErr = processVideoContent(page, contentURL, idx, content)
		}

		if processErr != nil {
			fmt.Printf("Warning: Failed to process %s: %v\n", content.Name, processErr)
			continue
		}

		items = append(items, item)
		time.Sleep(2 * time.Second)
	}

	output := OutputData{
		Name:      responseData.Course.Name,
		ItemCount: len(items),
		Items:     items,
	}

	outputPath := filepath.Join("jsons", responseData.Course.Name+".json")
	if err := saveJSON(outputPath, output); err != nil {
		return fmt.Errorf("failed to save output JSON: %v", err)
	}

	fmt.Printf("Successfully saved course data to %s\n", outputPath)
	return nil
}

func processHTMLContent(page *rod.Page, url string, idx int, content Content, courseDir string) (VideoItem, error) {
	if err := page.Navigate(url); err != nil {
		return VideoItem{}, fmt.Errorf("failed to navigate: %v", err)
	}

	err := page.WaitStable(5 * time.Second)
	if err != nil {
		return VideoItem{}, fmt.Errorf("page failed to stabilize: %v", err)
	}

	// Get the content container
	contentElement, err := page.Element(".course-player__content-inner")
	if err != nil {
		return VideoItem{}, fmt.Errorf("content container not found: %v", err)
	}

	// Get the HTML content
	htmlContent, err := contentElement.HTML()
	if err != nil {
		return VideoItem{}, fmt.Errorf("failed to get HTML content: %v", err)
	}

	// Create filename for this content
	filename := fmt.Sprintf("%d_%s.html", idx, sanitizeFilename(content.Name))
	filepath := filepath.Join(courseDir, filename)

	// Write the HTML content to a file
	err = os.WriteFile(filepath, []byte(htmlContent), 0644)
	if err != nil {
		return VideoItem{}, fmt.Errorf("failed to save HTML content: %v", err)
	}

	fmt.Printf("Saved HTML content to %s\n", filepath)

	return VideoItem{
		Index:      idx,
		Downloaded: true, // Mark as downloaded since we saved the HTML
		Name:       fmt.Sprintf("%d_%s", idx, content.Name),
		Type:       "text",
	}, nil
}

func processVideoContent(page *rod.Page, url string, idx int, content Content) (VideoItem, error) {
    if err := page.Navigate(url); err != nil {
        return VideoItem{}, fmt.Errorf("failed to navigate: %v", err)
    }

	err := page.WaitStable(5 * time.Second)
	if err != nil {
		return VideoItem{}, fmt.Errorf("page failed to stabilize: %v", err)
	}

    // Wait for any script tag to appear first
    err = rod.Try(func() {
        page.MustElement("script")
    })
    if err != nil {
        return VideoItem{}, fmt.Errorf("no scripts found on page: %v", err)
    }

    // Wait for iframe with a timeout
    iframe, err := page.Race().Element("iframe").Do()
    if err != nil {
        return VideoItem{}, fmt.Errorf("iframe not found: %v", err)
    }

    // Get the frame
    frame := iframe.MustFrame()

    // Execute JavaScript inside the frame to get the Wistia ID
    result, err := frame.Eval(`() => {
        const scriptElements = document.querySelectorAll('script');
        const dynamicParts = Array.from(scriptElements)
            .filter(s => s.src.startsWith('https://fast.wistia.com/embed/medias/'))
            .map(s => {
                const url = new URL(s.src);
                let dynamicPart = url.pathname.split('/')[3];
                return dynamicPart.replace('.jsonp', '');
            });
        return dynamicParts[0];
    }`)
    if err != nil {
        return VideoItem{}, fmt.Errorf("failed to evaluate frame scripts: %v", err)
    }

    dynamicPart := result.Value.String()
    if dynamicPart == "" {
        return VideoItem{}, fmt.Errorf("no Wistia video ID found")
    }

    fmt.Printf("Found video ID: %s\n", dynamicPart)

    return VideoItem{
        Index:       idx,
        DynamicPart: dynamicPart,
        Downloaded:  false,
        Name:        fmt.Sprintf("%d_%s", idx, content.Name),
        Type:        "video",
    }, nil
}


func initBrowser() *rod.Browser {
	u := launcher.New().
		Headless(false).
		Devtools(true).
		MustLaunch()

	browser := rod.New().
		ControlURL(u).
		SlowMotion(1 * time.Second). // Add some delay to see what's happening
		MustConnect()

	return browser
}

func parseResponseFile(path string) (*CourseData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var courseData CourseData
	if err := json.Unmarshal(data, &courseData); err != nil {
		return nil, err
	}

	return &courseData, nil
}

func saveJSON(path string, data interface{}) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	return encoder.Encode(data)
}