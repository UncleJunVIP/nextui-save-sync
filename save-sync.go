package main

import (
	"crypto/sha256"
	"fmt"
	_ "github.com/UncleJunVIP/certifiable"
	gaba "github.com/UncleJunVIP/gabagool/pkg/gabagool"
	"github.com/UncleJunVIP/nextui-pak-shared-functions/common"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var con config

type config struct {
	Bucket string `yaml:"bucket"`
	Prefix string `yaml:"prefix"`
	Region string `yaml:"region"`

	SaveDirectory string `yaml:"save_directory"`

	AccessKey string `yaml:"access_key"`
	SecretKey string `yaml:"secret_key"`

	LogLevel string `yaml:"log_level"`
}

func loadConfig(filePath string) (config, error) {
	var config config

	data, err := os.ReadFile(filePath)
	if err != nil {
		return config, fmt.Errorf("failed to read config file: %v", err)
	}

	err = yaml.Unmarshal(data, &config)
	if err != nil {
		return config, fmt.Errorf("failed to parse config file: %v", err)
	}

	if config.Region == "" {
		config.Region = "us-east-1"
	}

	if config.Bucket == "" {
		return config, fmt.Errorf("bucket name is required in config file")
	}
	if config.SaveDirectory == "" {
		return config, fmt.Errorf("save directory is required in config file")
	}

	if _, err := os.Stat(config.SaveDirectory); os.IsNotExist(err) {
		return config, fmt.Errorf("save directory does not exist: %s", config.SaveDirectory)
	}

	return config, nil
}

func createSession(config config) (*session.Session, error) {
	return session.NewSession(&aws.Config{
		Region:      aws.String(config.Region),
		Credentials: credentials.NewStaticCredentials(config.AccessKey, config.SecretKey, ""),
	})
}

func uploadSaves(config config) (int, int, error) {
	sess, err := createSession(config)
	if err != nil {
		return 0, 0, err
	}

	uploader := s3manager.NewUploader(sess)
	svc := s3.New(sess)
	count := 0
	skipped := 0

	return count, skipped, filepath.Walk(config.SaveDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error accessing path %s: %v", path, err)
		}

		if info.IsDir() || strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Calculate SHA-256 checksum of local file
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file %s: %v", path, err)
		}
		defer file.Close()

		// Calculate SHA-256 checksum
		h := sha256.New()
		if _, err := io.Copy(h, file); err != nil {
			return fmt.Errorf("failed to calculate checksum for %s: %v", path, err)
		}
		localChecksum := fmt.Sprintf("%x", h.Sum(nil))

		// Reset file pointer for later upload
		if _, err := file.Seek(0, 0); err != nil {
			return fmt.Errorf("failed to reset file pointer for %s: %v", path, err)
		}

		relPath, err := filepath.Rel(config.SaveDirectory, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %s: %v", path, err)
		}

		relPath = strings.ReplaceAll(relPath, "\\", "/")

		s3Key := config.Prefix
		if config.Prefix != "" && !strings.HasSuffix(config.Prefix, "/") {
			s3Key += "/"
		}
		s3Key += relPath

		// Check if file exists in S3 and compare checksums
		shouldUpload := true
		headInput := &s3.HeadObjectInput{
			Bucket: aws.String(config.Bucket),
			Key:    aws.String(s3Key),
		}

		result, err := svc.HeadObject(headInput)
		if err == nil {
			// File exists in S3, check if checksums match
			if result.Metadata != nil {
				if s3Checksum, ok := result.Metadata["X-Amz-Meta-Sha256-Checksum"]; ok && s3Checksum != nil {
					// Only upload if checksums are different
					shouldUpload = localChecksum != *s3Checksum
				}
			}
		}

		if !shouldUpload {
			skipped++
			log.Printf("Skipping %s (unchanged)", path)
			return nil
		}

		metadata := map[string]*string{
			"x-amz-meta-system-last-modified": aws.String(info.ModTime().Format(time.RFC3339)),
			"x-amz-meta-sha256-checksum":      aws.String(localChecksum),
		}

		_, err = uploader.Upload(&s3manager.UploadInput{
			Bucket:   aws.String(config.Bucket),
			Key:      aws.String(s3Key),
			Body:     file,
			Metadata: metadata,
		})

		if err != nil {
			return fmt.Errorf("failed to upload file %s to S3: %v", path, err)
		}

		log.Printf("Successfully uploaded %s to s3://%s/%s", path, config.Bucket, s3Key)

		count++
		return nil
	})
}

func downloadSaves(config config) (int, error) {
	sess, err := createSession(config)
	if err != nil {
		return 0, err
	}

	svc := s3.New(sess)

	count := 0

	err = svc.ListObjectsV2Pages(&s3.ListObjectsV2Input{
		Bucket: aws.String(config.Bucket),
		Prefix: aws.String(config.Prefix),
	}, func(page *s3.ListObjectsV2Output, lastPage bool) bool {
		for _, item := range page.Contents {
			if strings.HasSuffix(*item.Key, "/") {
				continue
			}

			relPath := *item.Key
			if config.Prefix != "" {
				if strings.HasPrefix(relPath, config.Prefix) {
					relPath = strings.TrimPrefix(relPath, config.Prefix)
					if !strings.HasSuffix(config.Prefix, "/") && strings.HasPrefix(relPath, "/") {
						relPath = strings.TrimPrefix(relPath, "/")
					}
				}
			}

			localPath := filepath.Join(config.SaveDirectory, relPath)

			localDir := filepath.Dir(localPath)
			if err := os.MkdirAll(localDir, 0755); err != nil {
				return false
			}

			file, err := os.Create(localPath)
			if err != nil {
				log.Printf("Error creating file %s: %v", localPath, err)
				continue
			}

			downloader := s3manager.NewDownloader(sess)
			_, err = downloader.Download(file, &s3.GetObjectInput{
				Bucket: aws.String(config.Bucket),
				Key:    item.Key,
			})

			file.Close()

			if err != nil {
				log.Printf("Error downloading %s: %v", *item.Key, err)
				os.Remove(localPath)
				continue
			}

			if item.LastModified != nil {
				os.Chtimes(localPath, *item.LastModified, *item.LastModified)
			}

			log.Printf("Successfully downloaded s3://%s/%s to %s", config.Bucket, *item.Key, localPath)
			count++
		}
		return true
	})

	if err != nil {
		return count, fmt.Errorf("error listing objects: %v", err)
	}

	return count, nil
}

func init() {
	gaba.InitSDL(gaba.GabagoolOptions{
		WindowTitle:    "Save Sync",
		ShowBackground: true,
	})
	common.SetLogLevel("ERROR")

	logger := common.GetLoggerInstance()

	configPath := "config.yml"
	if len(os.Args) > 1 {
		configPath = os.Args[1]
	}

	var err error

	con, err = loadConfig(configPath)
	if err != nil {
		gaba.ConfirmationMessage("Error Loading Configuration!\nCheck logs for more info.", []gaba.FooterHelpItem{
			{ButtonName: "B", HelpText: "Quit"},
		}, gaba.MessageOptions{})
		logger.Fatal("Error loading configuration!", zap.Error(err))
	}

	common.SetLogLevel(con.LogLevel)
}

func main() {
	defer common.CloseLogger()
	defer gaba.CloseSDL()

	common.GetLoggerInstance()

	mainMenuItems := []gaba.MenuItem{
		{
			Text:               "Upload Saves",
			Selected:           false,
			Focused:            false,
			NotMultiSelectable: false,
			Metadata:           "Upload",
		},
		{
			Text:               "Download Saves",
			Selected:           false,
			Focused:            false,
			NotMultiSelectable: false,
			Metadata:           "Download",
		},
	}

	options := gaba.DefaultListOptions("SaveSync", mainMenuItems)
	options.EnableAction = true
	options.FooterHelpItems = []gaba.FooterHelpItem{
		{ButtonName: "B", HelpText: "Quit"},
		{ButtonName: "A", HelpText: "Select"},
	}

	for {

		sel, err := gaba.List(options)
		if err != nil {
			log.Fatalf("Error displaying menu: %v", err)
		}

		if sel.IsNone() || sel.Unwrap().SelectedIndex == -1 {
			os.Exit(0)
		}

		switch sel.Unwrap().SelectedItem.Metadata.(string) {
		case "Upload":
			res, err := gaba.ProcessMessage("Uploading Saves...\nThis may take a second...", gaba.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
				uploaded, skipped, err := uploadSaves(con)

				return map[string]interface{}{
					"Uploaded": uploaded,
					"Skipped":  skipped,
				}, err
			})

			if err != nil {
				gaba.ProcessMessage("Error uploading saves!\nCheck the logs for more info.", gaba.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
					time.Sleep(2000 * time.Millisecond)
					return nil, nil
				})
			} else {
				message := ""

				if res.Result.(map[string]interface{})["Uploaded"].(int) != 0 {
					saveWord := "save"
					if res.Result.(map[string]interface{})["Uploaded"].(int) > 1 {
						saveWord += "s"
					}

					message += fmt.Sprintf("Uploaded %d %s!\n", res.Result.(map[string]interface{})["Uploaded"], saveWord)
				}

				if res.Result.(map[string]interface{})["Skipped"].(int) != 0 {
					saveWord := "save"
					if res.Result.(map[string]interface{})["Skipped"].(int) > 1 {
						saveWord += "s"
					}

					message += fmt.Sprintf("Skipped %d unchanged %s.\n", res.Result.(map[string]interface{})["Skipped"], saveWord)
				}

				gaba.ProcessMessage(message, gaba.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
					time.Sleep(2000 * time.Millisecond)
					return nil, nil
				})
			}
		case "Download":
			confirm, _ := gaba.ConfirmationMessage("Downloading saves can overwrite progress!\nProceed?", []gaba.FooterHelpItem{
				{ButtonName: "B", HelpText: "Cancel"},
				{ButtonName: "X", HelpText: "I Understand, Proceed"},
			}, gaba.MessageOptions{
				ConfirmButton: gaba.ButtonX,
			})

			if confirm.IsNone() {
				break
			}

			res, err := gaba.ProcessMessage("Downloading Saves...\nThis may take a second...", gaba.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
				return downloadSaves(con)
			})

			if err != nil {
				gaba.ProcessMessage("Error downloading saves!\nCheck the logs for more info.", gaba.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
					time.Sleep(2000 * time.Millisecond)
					return nil, nil
				})
			} else {
				gaba.ProcessMessage(fmt.Sprintf("Successfully downloaded %d saves!", res.Result.(int)), gaba.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
					time.Sleep(2000 * time.Millisecond)
					return nil, nil
				})
			}
		default:
			os.Exit(0)

		}
	}

}
