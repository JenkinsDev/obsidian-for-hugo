package main

import (
  "fmt"
  "flag"
  "os"
  "path"
  "regexp"
  "strings"
)

var help = flag.Bool("help", false, "Show help")
var vaultDir = flag.String("vault", "", "Path to Obsidian vault")
var outputDir = flag.String("content", "", "Path to Hugo content output directory (does not have to be content root)")
var clearHugoContentDir = flag.Bool("do-not-clear", true, "Clear Hugo content directory before converting")

var wikiLinkRegex = regexp.MustCompile(`\[\[(.*?)\]\]`)

func convertObsidianMarkdownToHugoMarkdown(contents []byte) []byte {
  contents = wikiLinkRegex.ReplaceAll(contents, []byte(`[$1]($1)`))
  return contents
}

// clear hugo content directory
// copy obsidian vault to hugo content directory
// convert obsidian markdown to hugo markdown
// convert obsidian yaml to hugo yaml
// convert obsidian images to hugo images
func copyObsidianToHugo(vaultDir string, contentDir string) error {
  var err error

  files, err := os.ReadDir(vaultDir)
  if err != nil {
    return err
  }

  for _, file := range files {
    name := file.Name()
    if name[0] == '.' {
      continue
    }

    vaultFullPath := path.Join(vaultDir, name)
    outputFullPath := path.Join(contentDir, name)

    if file.IsDir() {
      err = os.Mkdir(outputFullPath, 0755)
      if err != nil && !os.IsExist(err) {
        return err
      }

      err = copyObsidianToHugo(vaultFullPath, outputFullPath)
      if err != nil {
        return err
      }
    } else {
      contents, err := os.ReadFile(vaultFullPath)
      if strings.HasSuffix(name, ".md") {
        contents = convertObsidianMarkdownToHugoMarkdown(contents)
        if err != nil {
          return err
        }
      }

      err = os.WriteFile(outputFullPath, contents, 0644)
      if err != nil {
        return err
      }
    }
  }

  return nil
}

func ClearHugoContentDir(outputDir string) error {
  files, err := os.ReadDir(outputDir)
  if err != nil {
    return err
  }

  for _, file := range files {
    filePath := path.Join(outputDir, file.Name())

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

func ConvertObsidianToHugo(vaultDir string, outputDir string, clearHugoContentDir bool) error {
  var err error

  if clearHugoContentDir {
    err = ClearHugoContentDir(outputDir)
    if err != nil {
      return err
    }
  }

  err = copyObsidianToHugo(vaultDir, outputDir)
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

  err := ConvertObsidianToHugo(*vaultDir, *outputDir, *clearHugoContentDir)
  if err != nil {
    fmt.Println(err)
    os.Exit(1)
  }

  os.Exit(0)
}
