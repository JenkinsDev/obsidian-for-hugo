package main

import (
  "fmt"
  "flag"
  "os"
  "path"
  "regexp"
  "strings"

  "github.com/adrg/frontmatter"
  "gopkg.in/yaml.v2"
)

var help = flag.Bool("help", false, "Show help")
var vaultDir = flag.String("vault", "", "Path to Obsidian vault")
var outputDir = flag.String("content", "", "Path to Hugo content output directory (does not have to be content root)")
var clearHugoContentDir = flag.Bool("do-not-clear", true, "Clear Hugo content directory before converting")

var wikiLinkRegex = regexp.MustCompile(`\[\[(.*?)\]\]`)

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
  ClearOutputDir bool
  ContentProcessors []ContentProcessor
}

func convertObsidianYamlToHugoYaml(fileName string, contents []byte) []byte {
  var frontMatter FrontMatter

  stringReader := strings.NewReader(string(contents))
  rest, _ := frontmatter.Parse(stringReader, &frontMatter)

  if frontMatter.Title == "" {
    frontMatter.Title = strings.ReplaceAll(fileName, "#", "")
  }

  if frontMatter.Slug == "" {
    frontMatter.Slug = strings.ReplaceAll(fileName, " ", "-")
  }

  marshalled, _ := yaml.Marshal(frontMatter)
  return []byte(fmt.Sprintf("---\n%s---\n%s", marshalled, string(rest)))
}

func convertObsidianMarkdownToHugoMarkdown(fileName string, contents []byte) []byte {
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

/// Recursively copies files from the `fromDirPath` directory to the
/// `toDirPath` directory.
func copyObsidianToHugo(fromDirPath string, toDirPath string) ([]string, error) {
  var err error
  copiedPaths := []string{}

  files, err := os.ReadDir(fromDirPath)
  if err != nil {
    return copiedPaths, err
  }

  for _, file := range files {
    name := file.Name()
    if name[0] == '.' {
      continue
    }

    fromFullPath := path.Join(fromDirPath, name)
    outputFullPath := path.Join(toDirPath, name)

    if file.IsDir() {
      err = os.Mkdir(outputFullPath, 0755)
      if err != nil && os.IsNotExist(err) {
        return copiedPaths, err
      }

      nestedCopiedPaths, err := copyObsidianToHugo(fromFullPath, outputFullPath)
      if err != nil {
        return copiedPaths, err
      }

      copiedPaths = append(copiedPaths, nestedCopiedPaths...)
    } else {
      contents, err := os.ReadFile(fromFullPath)
      if err != nil {
        return copiedPaths, err
      }

      err = os.WriteFile(outputFullPath, contents, 0644)
      copiedPaths = append(copiedPaths, outputFullPath)
      if err != nil {
        return copiedPaths, err
      }
    }
  }

  return copiedPaths, nil
}

func clearDir(dir string) error {
  var err error

  files, err := os.ReadDir(dir)
  if err != nil && os.IsNotExist(err) {
    err = os.Mkdir(dir, 0755)
    if err != nil {
      return err
    }
  } else if err != nil {
    return err
  }

  for _, file := range files {
    filePath := path.Join(dir, file.Name())

    if file.IsDir() {
      err := os.RemoveAll(filePath)
      if err != nil {
        return err
      }
    } else {
      err := os.Remove(filePath)
      if err != nil {
        return err
      }
    }
  }

  return nil
}

func ConvertObsidianToHugo(config Config) error {
  var err error

  if config.ClearOutputDir {
    err = clearDir(config.OutputDir)
    if err != nil {
      return err
    }
  }

  copiedPaths, err := copyObsidianToHugo(config.VaultDir, config.OutputDir)
  if err != nil {
    return err
  }

  for _, path := range copiedPaths {
    if !strings.HasSuffix(path, ".md") {
      continue
    }

    contents, err := os.ReadFile(path)
    if err != nil {
      return err
    }

    fileName := path[len(config.OutputDir)+1:]
    fileName = strings.ReplaceAll(fileName, ".md", "")
    for _, processor := range config.ContentProcessors {
      contents = processor(fileName, contents)
    }

    os.WriteFile(path, contents, 0644)
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
    ClearOutputDir: *clearHugoContentDir,
    ContentProcessors: []ContentProcessor{
      convertObsidianYamlToHugoYaml,
      convertObsidianMarkdownToHugoMarkdown,
    },
  }

  err := ConvertObsidianToHugo(config)

  if err != nil {
    fmt.Println(err)
    os.Exit(1)
  }

  os.Exit(0)
}
