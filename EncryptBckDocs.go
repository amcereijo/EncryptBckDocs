package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"time"

	"golang.org/x/net/context"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
)

const configFileName = "config.json"

var configApp appConfig

type appConfig struct {
	FolderName string `json:"folderName"`
	LastUpdate string `json:"lastUpdate"`
}

// getClient uses a Context and Config to retrieve a Token
// then generate a Client. It returns the generated Client.
func getClient(ctx context.Context, config *oauth2.Config) *http.Client {
	cacheFile, err := tokenCacheFile()
	if err != nil {
		log.Fatalf("Unable to get path to cached credential file. %v", err)
	}
	tok, err := tokenFromFile(cacheFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(cacheFile, tok)
	}
	return config.Client(ctx, tok)
}

// getTokenFromWeb uses Config to request a Token.
// It returns the retrieved Token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var code string
	if _, err := fmt.Scan(&code); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(oauth2.NoContext, code)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// tokenCacheFile generates credential file path/filename.
// It returns the generated credential path/filename.
func tokenCacheFile() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	tokenCacheDir := filepath.Join(usr.HomeDir, ".credentials")
	os.MkdirAll(tokenCacheDir, 0700)
	return filepath.Join(tokenCacheDir,
		url.QueryEscape("EncryptBckDocs.json")), err
}

// tokenFromFile retrieves a Token from a given file path.
// It returns the retrieved Token and any read error encountered.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	t := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(t)
	defer f.Close()
	return t, err
}

// saveToken uses a file path to create a file and store the
// token in it.
func saveToken(file string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", file)
	f, err := os.Create(file)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

func findHolderFolder(folderName string, driveSrv *drive.Service) (file *drive.File, err error) {
	r, err := driveSrv.Files.List().Q("mimeType='application/vnd.google-apps.folder' and explicitlyTrashed=false").Fields("nextPageToken, files(id, name, mimeType)").Do()
	if err != nil {
		return nil, err
	}
	var folder *drive.File
	if len(r.Files) > 0 {
		for _, actualFile := range r.Files {
			// fmt.Printf("-- NAME: %s - ID: (%s) - TYPE:%s\n", actualFile.Name, actualFile.Id, actualFile.MimeType)
			if actualFile.Name == folderName {
				folder = actualFile
				//	break
			}
		}
		if folder == nil {
			errorString := fmt.Sprintf("No folder with name \"%s\"", folderName)
			err = errors.New(errorString)
		}
	} else {
		err = errors.New("No folders")
	}
	return folder, err
}

func findUploadFileInDrive(fileName string, parentID string, driveSrv *drive.Service) (fileToUpload *drive.File, err error) {
	r, err := driveSrv.Files.List().Q("'" + parentID + "' in parents and explicitlyTrashed=false and name='" + fileName + "'").Fields("files(id, name)").Do()
	if err != nil {
		return nil, err
	}
	if len(r.Files) > 0 {
		fileToUpload = r.Files[0]
	}
	return fileToUpload, err
}

func updateLastUpdateAppConfig() {
	configApp.LastUpdate = time.Now().String()
	saveConfigJSONFile()
}

func updateFileInDrive(driveFileToUpload *drive.File, goFile *os.File, driveSrv *drive.Service) (err error) {
	fmt.Printf("Upate existing file %s\n!!", driveFileToUpload.Name)
	driveFileToUpdate := &drive.File{
		Name: filepath.Base(driveFileToUpload.Name),
	}

	_, err = driveSrv.Files.Update(driveFileToUpload.Id, driveFileToUpdate).Media(goFile).Do()
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("Updated file \"%s\"!!\n", driveFileToUpload.Name)
		updateLastUpdateAppConfig()
	}

	return err
}

func uploadNewFileToDrive(folderFile *drive.File, fileToUploadName string, fileToUploadURL string, goFile *os.File, driveSrv *drive.Service) (err error) {
	parents := []string{folderFile.Id}
	driveFileToUpload := &drive.File{
		Parents: parents,
		Name:    filepath.Base(fileToUploadName),
	}
	_, err = driveSrv.Files.Create(driveFileToUpload).Media(goFile).Do()
	if err != nil {
		panic(err)
	} else {
		fmt.Printf("Uploaded file \"%s\" to \"%s\" !!\n", fileToUploadName, folderFile.Name)
		updateLastUpdateAppConfig()
	}
	return err
}

func loadConfig() (config appConfig, err error) {
	configFileContent, err := os.Open(configFileName)
	if err != nil {
		return config, err
	}
	defer configFileContent.Close()

	jsonParser := json.NewDecoder(configFileContent)
	err = jsonParser.Decode(&config)
	if err != nil {
		return config, err
	}

	return config, nil
}

func saveConfigJSONFile() {
	//save json file
	jsonContent, err := json.Marshal(configApp)
	if err != nil {
		log.Printf("ERROR! Cannot create config file: %v ", err)
	} else {
		ioutil.WriteFile(configFileName, jsonContent, 0644)
	}
}

func createConfig() (config appConfig) {
	// create config file
	folderName := "EncryptBckDoc"
	var inputFolderName string
	fmt.Print("Name for the folder to save files (default: EncryptBckDoc): ")
	fmt.Scanln(&inputFolderName)
	if inputFolderName != "" {
		folderName = inputFolderName
	}
	configApp = appConfig{
		FolderName: folderName,
	}
	//save json file
	saveConfigJSONFile()

	return configApp
}

func createFolderInDrive(folderName string, driveSrv *drive.Service) (folderFile *drive.File, err error) {
	log.Printf("Error finding %s : %v\n", folderName, err)
	// create folder
	fileMeta := &drive.File{
		Name:     folderName,
		MimeType: "application/vnd.google-apps.folder",
	}
	folderFile, err = driveSrv.Files.Create(fileMeta).Do()

	return folderFile, err
}

func main() {
	context := context.Background()

	b, err := ioutil.ReadFile("client_secret.json")
	if err != nil {
		log.Fatalf("Unable to read client secret file: %v", err)
	}

	// If modifying these scopes, delete your previously saved credentials
	// at ~/.credentials/drive-go-quickstart.json
	config, err := google.ConfigFromJSON(b, drive.DriveScope)
	if err != nil {
		log.Fatalf("Unable to parse client secret file to config: %v", err)
	}
	client := getClient(context, config)

	driveSrv, err := drive.New(client)
	if err != nil {
		log.Fatalf("Unable to retrieve drive Client %v", err)
	}

	configApp, err = loadConfig()
	if err != nil {
		configApp = createConfig()
	}

	fmt.Printf("Looking for folder \"%s\"...\n", configApp.FolderName)

	folderFile, err := findHolderFolder(configApp.FolderName, driveSrv)
	if err != nil {
		folderFile, err = createFolderInDrive(configApp.FolderName, driveSrv)

		if err != nil {
			panic(err)
		} else {
			fmt.Printf("Created folder \"%s\" for files!!\n", configApp.FolderName)
		}
	}

	fmt.Printf("Found folder %s - ID: (%s) - TYPE:%s\n", folderFile.Name, folderFile.Id, folderFile.MimeType)

	fileToUploadName := "Example.txt"
	fileToUploadURL := "./Example.txt"
	goFile, err := os.Open(fileToUploadURL)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}

	var driveFileToUpload *drive.File
	driveFileToUpload, err = findUploadFileInDrive(fileToUploadName, folderFile.Id, driveSrv)
	if err != nil {
		log.Fatalf("Error checking if file \"%s\" already exists", fileToUploadName)
	}

	if driveFileToUpload != nil {
		updateFileInDrive(driveFileToUpload, goFile, driveSrv)
	} else {
		uploadNewFileToDrive(folderFile, fileToUploadName, fileToUploadURL, goFile, driveSrv)
	}

}
