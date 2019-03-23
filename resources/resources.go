package resources

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"golang.org/x/crypto/openpgp"
)

const (
	MajorVersion       = "v2"
	releaseURLTemplate = "https://api.github.com/repos/bananaboatbot/resources.%s/releases/latest"
	keyringData        = `-----BEGIN PGP PUBLIC KEY BLOCK-----

mQINBFySbYcBEACb/pAr8GXAP05Vw/WHoQtKHeKI0bPiYqt2u1RHDQKBMT4WewqK
39r7NDC3kdkf4Z0nYyev5M6we2NbNJ8+gYnnjAQH39kkiifo6Ev/FsMJThXWgh6S
386eFlleES2J8StOOTKi/0gzTE8Rl/V6ffE4lOY/I2oZHrO7p1L2NrE62k+5NOrU
DNkb4JufUjDdyWqVS9qZFa6n/jz/PFiW+JVTlmOT3crgA2e+cjdjiawzTVmCAMIg
mSEwPySj5oVVAHkNsoB5JReUNUVwNf6kG59Op76SHkcepYofwAhuCEsQgZCCc0BV
gENjQa8O1cx3YRfFcxwX+YlUvp9nFKe0qd9Bq3gXnh1DOylAtnWgk4fNeKF/2gTD
uCI47JF+T6P/Gxcp+OjbvoEvc3dPe2TaGoxLrIELOZyxdyz0Y7VoSO+GdjHuX3Oj
Z7BivEx1LUyRbs+tG/d6g0KpAmQIolayAFc+3173twwcLC/iPa7Pq9gddTXHPDRm
GIw4slZs193/UGL2M2a2GHQoeSTVGLpVEF4/1Bo17tFDUAtgX1q3fBP5mhbrujwI
ut3h8Q1Zo2GGK1ThdOCmleHqzFiWLwYD4X+5E0iV/1rrj/4tGWuMlm0F5DY5h9gW
IcdV/9DkUVcMUcGzdmRuF6Rm+ZWosjAJgbevaKFu2yT0MCa12f2myPouIQARAQAB
tB9BbmRyZXcgTGV3aXMgPG5lcmZAanVkby56YS5vcmc+iQJUBBMBCAA+FiEE6BoZ
holmW2MUdjdInz9fSD7WCDQFAlySbYcCGwMFCQPCZwAFCwkIBwIGFQoJCAsCBBYC
AwECHgECF4AACgkQnz9fSD7WCDQ81A//RMWJ1Ozovs3EWTuVf9vTBbjP7S6iAwz+
nfyTUeGOtll74P/bfsuZpNTj979EJXzYA1drMnjoW6j1fczZ7NJeQCHUHSBID6mL
GzspD/BKqy/WV+r8iyOOHYseFR7N+KMpimMXVCcL0hBZHX1KgNtG4zQWJfxt377i
f0EWZ9xAV48XSnClYT/h1vthG7f9BP+QLoy2seUYxbyHL9Jwz7109Gb6bSaDvEE5
83HGZE2cbAlCiXJi5ad1fI2eak3bc28W7hsvw8h038E/lB4RU63VA22zIZ8QG9WO
WbsZhZbpWW9ZPXsB/GkY11H0qK7WWPQ+bmFmXUGlYWYan4yd6ydGhMNsD6EgKUt8
0yf5nR8sAUkqfogOdgAP055Ghw2MzXUNBuGZ2V7pBkIHjF5iUZB0G3Y76n+3EgpN
XH1Fhh9X+phkS2K5UC5YvtjwlGhQ7WhzUtuhQzTeLO3k6KN0/p+fUlhURBUd7kPh
RgTjJy2Nn7n/Q9F2pqstlQqWjB8pcQwcu6ooA2N8Ht4LwC2Zse0uHRjXPWUlJCSF
YzOnQy1+21eltjBjkWfHu/GH9WhBt6Zhy7txZps9gu+p0QM0JTBqJpx7zCcVNzRV
6Nea+iPBZQBWRZSh6swtv6l/CieSxQFHvHsIeE/cM7MqKgEh7sKRdkW3111lDWBK
Iqi0811YKeG5Ag0EXJJthwEQALpbA45fuPzZ65TcYLM7bOYefcrUao3nQ+8ivvSk
wUXkxlte2ugtIr/inweq215UZOD7LqqqrGTYdUm6Cl6wL+27LpQA1NVENaO2+bel
H9yhqNdT525wT77LgJ6Syb+f5GQTNDiLyu1hTcGoXLebZpZnZ4kSNjpKPjsP6M5J
YpYrIon8WFV1AurJqzoMmagTOMjv2BPFgEe4eJMluemavhKP8441jgVPOft2pzsH
gCqtMlLfdedHEycJlFvhv3A24deGU5BPBlXB+WT30OuID0/JT8+0c5I8m1R44SUX
6aq9hbHZT1Tz67q+AhMsGo5+X4+CVwz5zg4WJI78KDwNbWdZc/snemcu0BmfwxGw
bQufPSeO2E8Qpaa2A06JZfh7g95VNUN1zSbFpmSHmr+jU9rdHC5fvMTANtS4v8yx
wgjx3eC8y/WTzA94SBhyXr5SYmeJHaz6mbfsUHRqL0P5PQgDJD4hOo6C4VnaU6sV
wOVPK0Yy6LVLl0m3dZfuU/7SovUnak2HH9Ltz3lgGYYzIp9fISwLr5NXHTgmlbcF
6clQNG74fb/ewd/1DVvsFmChGNCMzFwE6KOqyK7YdBcEYlEkd08YobmPmeFXwzjD
Nd+m6eqT+6OX4bBp21xkQDkSkaaxHt43QSz8e71mFjw9vJJBK3HmyaA5VmVxTMwT
74xzABEBAAGJAjwEGAEIACYWIQToGhmGiWZbYxR2N0ifP19IPtYINAUCXJJthwIb
DAUJA8JnAAAKCRCfP19IPtYINHjlD/0aozMeCju5JGKWwCoJ2OX1E9tLUJ8MeBMh
9sYR6IhJlzt4TpbyQqQleElU9bXGCfE0ZS6sVYV7Nx1rJzZjIc8MPJb9BYHma/bc
FPuCTojWEIkkBBri56y9pr81Kxa+Dko7praIrWVMA1ccvORFmeM2ZTHaKuJjnWIr
OUlNCo50L6QfPF4za5mltOz8TOM5H4o3eo31IwsqLDABrx8sobejFD0sHlS78uvl
P46f4e85AL+2E3/FsRg2oCGx/fXPRLJkG6s3Qr9FgqQKQcemef3O7fbAH2NhvvAf
PVcWOoTIut30OUGMddWVzFKrkPC5dO55jnmJcTlvf89OilaRsIF69fUYyIBdEGEI
OYF6CM5x3oxEOrhDCmeNsCgSml0kfFpI8Ar4LDeCjKanXx0P/aaoFtusNkICrfFM
oajxHtqmb0T6+tHzhMw69rP1Dq4NdOgHs69SRmhB8ZGLwgVRAubtATGfBIrC1WaI
6y+1PbHOQdOvWBiAwAJJgJc1kG3wm2E9sYivylLWpDFs4S8lNZoGsQPRmnDM1PwC
XrY8DAdZ7/DEKfI1+NH5eGi6CAjtvD2o6fblOPrTMv/BlmTqusIWPAsNOc3m29/j
VBs4Oo97Y3AKNprq19w6DELegG89m800DL0WPcerrGquyn4VLXxybdxKBYJweJB3
LPR5VUbSkg==
=AR7F
-----END PGP PUBLIC KEY BLOCK-----`
)

type githubRelease struct {
	TagName    string        `json:"tag_name"`
	Assets     []githubAsset `json:"assets"`
	TarballURL string        `json:"tarball_url"`
}

type githubAsset struct {
	Name        string `json:"name"`
	DownloadURL string `json:"browser_download_url"`
}

func GetResources() {
	// Get resources directory
	resourcesDir, err := GetResourcesDirectory()
	if err != nil {
		log.Printf("Couldn't get temporary directory: %s", err)
		return
	}
	// Create resources directory if necessary
	err = os.MkdirAll(resourcesDir, 0700)
	if err != nil {
		log.Printf("Couldn't create resources directory: %s", err)
		return
	}
	// Create temporary directory for downloads
	dir, err := ioutil.TempDir("", "bananaboatbot")
	if err != nil {
		log.Printf("Couldn't create temporary directory: %s", err)
		return
	}
	defer os.RemoveAll(dir)
	// Create HTTP client
	client := &http.Client{
		Timeout: time.Second * 60,
	}
	// Get latest tarball & signature URLs
	urlList, err, tagName := getDownloadURLs(client)
	if err != nil {
		log.Printf("Couldn't get download URLs: %s", err)
		return
	}
	// Download files, transform urlList to list of paths on disk
	for i, dl := range urlList {
		p, err := downloadToDirectory(client, dir, dl)
		if err != nil {
			log.Printf("Download of %s failed: %s", dl, err)
			return
		}
		urlList[i] = p
	}
	// Verify signature
	err = verifySignature(urlList[0], urlList[1])
	if err != nil {
		log.Printf("Failed to validate signature: %s", err)
		return
	}
	err = installResources(urlList[0], resourcesDir)
	if err != nil {
		log.Printf("Failed to install resources: %s", err)
		return
	}
	log.Printf("Installed resources %s", tagName)
}

// GetResourcesDirectory returns our resources directory
func GetResourcesDirectory() (string, error) {
	// Get user cache directory...
	cacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	// Construct our directory relative to cache directory
	return path.Join(cacheDir, "bananaboatbot"), nil
}

// trimPath removes the first element from the path
func trimPath(filePath string) string {
	idx := strings.Index(filePath, "/")
	if idx == -1 {
		return ""
	}
	return filePath[idx+1:]
}

// installResources extracts resources tarball to its destination
func installResources(tarPath string, resourcesDir string) error {
	// Open file
	in, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	// Create gzip reader
	gzin, err := gzip.NewReader(in)
	if err != nil {
		return err
	}
	// Create tar reader
	rdr := tar.NewReader(gzin)
	// Extract files...
	for {
		hdr, err := rdr.Next()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			filePath := trimPath(hdr.Name)
			if filePath == "" {
				break
			}
			finalPath := path.Join(resourcesDir, filePath)
			err := os.MkdirAll(finalPath, 0700)
			if err != nil {
				return err
			}
		case tar.TypeReg:
			filePath := trimPath(hdr.Name)
			if filePath == "" {
				break
			}
			finalPath := path.Join(resourcesDir, filePath)
			f, err := os.OpenFile(finalPath, os.O_CREATE|os.O_RDWR, 0600)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, rdr)
			closeErr := f.Close()
			if err != nil {
				return err
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
	return nil
}

// verifySignature checks detached signature against content
func verifySignature(tarPath string, sigPath string) error {
	// Create keyring from embedded key
	keyring, err := openpgp.ReadArmoredKeyRing(strings.NewReader(keyringData))
	if err != nil {
		return err
	}
	// Open tarfile
	tarFile, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer tarFile.Close()
	// Open signature file
	sigFile, err := os.Open(sigPath)
	if err != nil {
		return err
	}
	defer sigFile.Close()
	// Verify signature
	_, err = openpgp.CheckArmoredDetachedSignature(keyring, tarFile, sigFile)
	return err

}

// downloadToDirectory fetches a file over HTTP and writes it to some directory
func downloadToDirectory(client *http.Client, dir string, dl string) (string, error) {
	// Create file to write to
	out, err := os.Create(path.Join(dir, path.Base(dl)))
	if err != nil {
		return "", err
	}
	name := out.Name()
	defer out.Close()
	// Create HTTP request
	req, err := http.NewRequest("GET", dl, nil)
	if err != nil {
		return name, err
	}
	// Send request
	res, err := client.Do(req)
	if err != nil {
		return name, err
	}
	// Write output to file
	_, err = io.Copy(out, res.Body)
	return name, err
}

// getDownloadURLs returns tarball URL & signature URL or error + TagName
func getDownloadURLs(client *http.Client) ([]string, error, string) {
	// Make list of two URLs to return
	downloadURLs := make([]string, 2)
	// Construct URL for metainformation about latest release
	releaseURL := fmt.Sprintf(releaseURLTemplate, MajorVersion)
	// Create structure to decode JSON into
	release := githubRelease{}
	// Create HTTP request
	req, err := http.NewRequest("GET", releaseURL, nil)
	if err != nil {
		return downloadURLs, err, ""
	}
	// Send request
	res, err := client.Do(req)
	if err != nil {
		return downloadURLs, err, ""
	}
	// Decode response
	dec := json.NewDecoder(res.Body)
	err = dec.Decode(&release)
	if err != nil {
		return downloadURLs, err, ""
	}
	// Set URL of tarball to result
	downloadURLs[0] = release.TarballURL
	// Construct name of asset to search for
	wantFile := fmt.Sprintf("%s.tar.gz.asc", release.TagName)
	// Try find asset
	for _, asset := range release.Assets {
		if asset.Name == wantFile {
			downloadURLs[1] = asset.DownloadURL
			break
		}
	}
	// Return error if we didn't find a signature
	if len(downloadURLs[1]) == 0 {
		return downloadURLs, fmt.Errorf("Couldn't find asset named %s", wantFile), release.TagName
	}
	return downloadURLs, nil, release.TagName
}
