package main

import (
  "fmt"
  "flag"
  "io/ioutil"
  "os"
  "os/exec"
  "path"
  "regexp"
  "strings"
  "strconv"
  "sync"
  "time"

  "github.com/adrg/frontmatter"
  "github.com/creack/pty"
  "gopkg.in/yaml.v2"
)

var help = flag.Bool("help", false, "Show help")
var vaultDir = flag.String("vault-path", "", "Path to Obsidian vault")
var outputDir = flag.String("content-path", "", "Path to Hugo content output directory (does not have to be content root)")
var wg sync.WaitGroup

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

type File struct {
  Path string
  Name string
  Title string
  Contents []byte
}

type FrontMatterProcessor = func(Config, File, *FrontMatter)
type ContentProcessor = func(Config, File, []byte) []byte

type Config struct {
  VaultDir string
  OutputDir string
  FrontMatterProcessors []FrontMatterProcessor
  ContentProcessors []ContentProcessor
}

func addFallbackFrontMatterTitle(config Config, file File, frontMatter *FrontMatter) {
  if frontMatter.Title == "" {
    frontMatter.Title = file.Title
  }
}

func addFallbackFrontMatterSlug(config Config, file File, frontMatter *FrontMatter) {
  if frontMatter.Slug == "" {
    slugifiedTitle := slugifyRegex.ReplaceAllString(frontMatter.Title, "-")
    frontMatter.Slug = strings.ToLower(slugifiedTitle)
  }
}

func attemptGitDate(config Config, file File, frontMatter *FrontMatter) {
  gitRoot, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
  if err != nil {
    return
  }

  command := exec.Command("git", "--no-pager", "log", "-1", "--format=%ad", "--date=unix",  "--", file.Path)
  command.Dir = string(gitRoot)

  // git log will only output to STDOUT if it's a terminal, so we need to create a PTY to capture the output
  ptmx, err := pty.Start(command)
  if err != nil {
    return
  }

  defer ptmx.Close()
  output, err := ioutil.ReadAll(ptmx)

  if err != nil {
    timestamp := strings.TrimSpace(string(output))
    i, _ := strconv.ParseInt(timestamp, 10, 64)
    frontMatter.Date = time.Unix(i, 0).Format(time.RFC3339)
  }
}

func attemptFileDate(config Config, file File, frontMatter *FrontMatter) {
  fileInfo, err := os.Stat(file.Path)
  if err != nil {
    return
  }

  frontMatter.Date = fileInfo.ModTime().Format(time.RFC3339)
}

func addFallbackFrontMatterDate(config Config, file File, frontMatter *FrontMatter) {
  if frontMatter.Date == "" {
    attemptGitDate(config, file, frontMatter)
  }

  if frontMatter.Date == "" {
    attemptFileDate(config, file, frontMatter)
  }
  
  if frontMatter.Date == "" {
    frontMatter.Date = time.Now().Format(time.RFC3339)
  }
}

func convertWikiLinks(config Config, file File, contents []byte) []byte {
  contents = wikiLinkRegex.ReplaceAllFunc(contents, func(match []byte) []byte {
    link := string(match[2:len(match)-2])

    if strings.Contains(link, "#") {
      heading := link[strings.Index(link, "#")+1:]
      heading = strings.ReplaceAll(heading, " ", "-")
      heading = strings.ToLower(heading)

      link = link[0:strings.Index(link, "#")]
      return []byte(fmt.Sprintf("[%s]({{< ref \"%s#%s\" >}})", link, link, heading))
    }

    return []byte(fmt.Sprintf("[%s]({{< ref \"%s\" >}})", link, link))
  })

  return contents
}

/// Parses the frontmatter from the file, returns the frontmatter and the rest of the
/// file's contents.
func parseFrontMatter(config Config, file File) (FrontMatter, []byte, error) {
  var frontMatter FrontMatter

  stringReader := strings.NewReader(string(file.Contents))
  rest, err := frontmatter.Parse(stringReader, &frontMatter)
  if err != nil {
    return frontMatter, rest, err
  }

  return frontMatter, rest, nil
}

func marshalFrontMatter(frontMatter *FrontMatter) []byte {
  marshalled, _ := yaml.Marshal(frontMatter)
  return []byte(fmt.Sprintf("---\n%s---", marshalled))
}

func convertFile(config Config, fromPath string, toPath string) error {
  var file File
  defer wg.Done()

  contents, err := os.ReadFile(fromPath)
  if err != nil {
    return err
  }

  file.Path = fromPath
  file.Contents = contents
  file.Name = path.Base(fromPath)
  file.Title = strings.ReplaceAll(strings.ReplaceAll(file.Name, ".md", ""), "#", "")

  frontMatter, body, err := parseFrontMatter(config, file)
  if err != nil {
    return err
  }

  for _, processor := range config.ContentProcessors {
    body = processor(config, file, body)
  }

  for _, processor := range config.FrontMatterProcessors {
    processor(config, file, &frontMatter)
  }

  marshalledFrontMatter := marshalFrontMatter(&frontMatter)
  file.Contents = []byte(fmt.Sprintf("%s\n%s", marshalledFrontMatter, string(body)))

  err = os.WriteFile(toPath, file.Contents, 0644)
  if err != nil {
    return err
  }

  return nil
}

func convertAllRecursively(config Config, fromDirPath string, toDirPath string) error {
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

      err := convertAllRecursively(config, fromFullPath, toFullPath)
      if err != nil {
        return err
      }
    } else {
      wg.Add(1)
      go convertFile(config, fromFullPath, toFullPath)
    }
  }

  return nil
}

func ConvertObsidianToHugo(config Config) error {
  // clean up the output directory
  err := os.RemoveAll(config.OutputDir)
  if err != nil {
    return err
  }

  err = os.MkdirAll(config.OutputDir, 0755)
  if err != nil {
    return err
  }

  return convertAllRecursively(config, config.VaultDir, config.OutputDir)
}

func main() {
  flag.Parse()

  if *help {
    flag.PrintDefaults()
    os.Exit(0)
  } else if *vaultDir == "" {
    fmt.Println("Obsidian Vault path is required")
    os.Exit(1)
  } else if *outputDir == "" {
    fmt.Println("Hugo Content path is required")
    os.Exit(1)
  }

  err := ConvertObsidianToHugo(Config{
    VaultDir: *vaultDir,
    OutputDir: *outputDir,
    FrontMatterProcessors: []FrontMatterProcessor{
      addFallbackFrontMatterTitle,
      addFallbackFrontMatterSlug,
      addFallbackFrontMatterDate,
    },
    ContentProcessors: []ContentProcessor{
      convertWikiLinks,
    },
  })

  if err != nil {
    fmt.Println(err)
    os.Exit(1)
  }

  wg.Wait()

  os.Exit(0)
}
