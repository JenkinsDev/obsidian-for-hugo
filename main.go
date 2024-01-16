package main

import (
  "fmt"
  "flag"
  "os"
  "path"
  "regexp"
  "strings"
  "time"

  "github.com/adrg/frontmatter"
  "gopkg.in/yaml.v2"
)

var help = flag.Bool("help", false, "Show help")
var vaultDir = flag.String("vault-path", "", "Path to Obsidian vault")
var outputDir = flag.String("content-path", "", "Path to Hugo content output directory (does not have to be content root)")

var wikiLinkRegex = regexp.MustCompile(`\[\[(.*?)\]\]`)
var slugifyRegex = regexp.MustCompile(`[^a-zA-Z0-9]`)

type FrontMatter struct {
  Title string `yaml:"title"`
  Date string `yaml:"date"`
  Draft bool `yaml:"draft"`
  Tags []string `yaml:"tags"`
  Categories []string `yaml:"categories"`
  Slug string `yaml:"slug"`
}

type ContentProcessor = func(string, []byte) []byte
type Config struct {
  VaultDir string
  OutputDir string
  ContentProcessors []ContentProcessor
}

func convertFrontMatter(fileName string, contents []byte) []byte {
  var frontMatter FrontMatter

  stringReader := strings.NewReader(string(contents))
  rest, _ := frontmatter.Parse(stringReader, &frontMatter)

  if frontMatter.Title == "" {
    frontMatter.Title = strings.ReplaceAll(fileName, "#", "")
  }

  if frontMatter.Slug == "" {
    slugifiedTitle := slugifyRegex.ReplaceAllString(frontMatter.Title, "-")
    frontMatter.Slug = strings.ToLower(slugifiedTitle)
  }

  if frontMatter.Date == "" {
    fileInfo, err := os.Stat(fileName)
    if err != nil {
      frontMatter.Date = time.Now().Format(time.RFC3339)
    } else {
      frontMatter.Date = fileInfo.ModTime().Format(time.RFC3339)
    }
  }

  marshalled, _ := yaml.Marshal(frontMatter)
  return []byte(fmt.Sprintf("---\n%s---\n%s", marshalled, string(rest)))
}

func convertContent(fileName string, contents []byte) []byte {
  contents = wikiLinkRegex.ReplaceAllFunc(contents, func(match []byte) []byte {
    link := string(match[2:len(match)-2])

    if strings.Contains(link, "#") {
      link = link[0:strings.Index(link, "#")]
      heading := link[strings.Index(link, "#")+1:]
      heading = strings.ReplaceAll(heading, " ", "-")
      heading = strings.ToLower(heading)
      return []byte(fmt.Sprintf("[%s]({{< ref \"%s#%s\" >}})", link, link, heading))
    }

    return []byte(fmt.Sprintf("[%s]({{< ref \"%s\" >}})", link, link))
  })

  return contents
}

func convertFile(fromPath string, toPath string, contentProcessors []ContentProcessor) {
  contents, err := os.ReadFile(fromPath)
  if err != nil {
    return
  }

  fileName := path.Base(fromPath)
  fileName = strings.ReplaceAll(fileName, ".md", "")

  for _, processor := range contentProcessors {
    contents = processor(fileName, contents)
  }

  err = os.WriteFile(toPath, contents, 0644)
  if err != nil {
    return
  }
}

func convertAllRecursively(fromDirPath string, toDirPath string, contentProcessors []ContentProcessor) error {
  var err error

  files, err := os.ReadDir(fromDirPath)
  if err != nil {
    return err
  }

  for _, file := range files {
    name := file.Name()
    if name[0] == '.' {
      continue
    }

    fromFullPath := path.Join(fromDirPath, name)
    toFullPath := path.Join(toDirPath, name)

    if file.IsDir() {
      err = os.Mkdir(toFullPath, 0755)
      if err != nil && os.IsNotExist(err) {
        return err
      }

      err := convertAllRecursively(fromFullPath, toFullPath, contentProcessors)
      if err != nil {
        return err
      }
    } else {
      go convertFile(fromFullPath, toFullPath, contentProcessors)
    }
  }

  return nil
}

func ConvertObsidianToHugo(config Config) error {
  err := convertAllRecursively(config.VaultDir, config.OutputDir, config.ContentProcessors)
  if err != nil {
    return err
  }

  return nil
}

func main() {
  flag.Parse()

  if *help {
    flag.PrintDefaults()
    os.Exit(0)
  }

  if *vaultDir == "" {
    fmt.Println("Obsidian Vault path is required")
    os.Exit(1)
  }

  if *outputDir == "" {
    fmt.Println("Hugo Content path is required")
    os.Exit(1)
  }

  config := Config{
    VaultDir: *vaultDir,
    OutputDir: *outputDir,
    ContentProcessors: []ContentProcessor{
      convertFrontMatter,
      convertContent,
    },
  }

  err := ConvertObsidianToHugo(config)

  if err != nil {
    fmt.Println(err)
    os.Exit(1)
  }

  os.Exit(0)
}
