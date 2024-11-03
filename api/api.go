package api

import (
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/haveachin/infrared"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"
)

// APIConfig holds the API configuration including the Bearer token
type APIConfig struct {
	BearerToken string `json:"bearerToken"`
}

var config APIConfig

// authenticateMiddleware checks for valid Bearer token in Authorization header
func authenticateMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		
		if authHeader == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "No authorization header provided",
			})
			return
		}

		// Check if the header starts with "Bearer "
		headerParts := strings.Split(authHeader, " ")
		if len(headerParts) != 2 || headerParts[0] != "Bearer" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid authorization header format",
			})
			return
		}

		token := headerParts[1]
		if token != config.BearerToken {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(map[string]string{
				"error": "Invalid token",
			})
			return
		}

		next.ServeHTTP(w, r)
	}
}

// LoadConfig loads the API configuration from file
func LoadConfig(configFile string) error {
	data, err := ioutil.ReadFile(configFile)
	if err != nil {
		return err
	}

	err = json.Unmarshal(data, &config)
	if err != nil {
		return err
	}

	if config.BearerToken == "" {
		return fmt.Errorf("bearer token not found in config file")
	}

	return nil
}

// ListenAndServe Start Webserver
func ListenAndServe(configPath string, apiBind string, apiConfigPath string) {
	// Load API configuration
	err := LoadConfig(apiConfigPath)
	if err != nil {
		log.Fatalf("Failed to load API config: %v", err)
	}

	log.Println("Starting WebAPI on " + apiBind)
	router := mux.NewRouter()

	// Apply authentication to all routes
	router.HandleFunc("/", authenticateMiddleware(getHome())).Methods("GET")
	router.HandleFunc("/proxies", authenticateMiddleware(getProxies(configPath))).Methods("GET")
	router.HandleFunc("/proxies/{name}", authenticateMiddleware(getProxy(configPath))).Methods("GET")
	router.HandleFunc("/proxies/{name}", authenticateMiddleware(addProxyWithName(configPath))).Methods("POST")
	router.HandleFunc("/proxies/{name}", authenticateMiddleware(removeProxy(configPath))).Methods("DELETE")

	if infrared.Config.Tableflip.Enabled {
		listen, err := infrared.Upg.Listen("tcp", apiBind)
		if err != nil {
			log.Printf("Failed to start API listener: %s", err)
			return
		}
		err = http.Serve(listen, router)
		if err != nil {
			log.Printf("Failed to start serving API: %s", err)
			return
		}
	} else {
		err := http.ListenAndServe(apiBind, router)
		if err != nil {
			log.Printf("Failed to start serving API: %s", err)
			return
		}
	}
}

// getHome
func getHome() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {}
}

// getProxies
func getProxies(configPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var configs []string

		files, err := ioutil.ReadDir(configPath)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		for _, file := range files {
			configs = append(configs, strings.Split(file.Name(), ".json")[0])
		}

		err = json.NewEncoder(w).Encode(&configs)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

// getProxy
func getProxy(configPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileName := mux.Vars(r)["name"] + ".json"

		jsonFile, err := os.Open(configPath + "/" + fileName)
		defer jsonFile.Close()
		if err != nil {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		config, err := ioutil.ReadAll(jsonFile)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		_, err = w.Write(config)
		if err != nil {
			log.Println(err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
	}
}

// addProxyWithName respond to post proxy request
func addProxyWithName(configPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileName := mux.Vars(r)["name"] + ".json"

		rawData, err := ioutil.ReadAll(r.Body)
		if err != nil || string(rawData) == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		jsonIsValid := checkJSONAndRegister(rawData, fileName, configPath)
		if jsonIsValid {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("{'success': true, 'message': 'the proxy has been added succesfully'}"))
			return
		} else {
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte("{'success': false, 'message': 'domainNames and proxyTo could not be found'}"))
			return
		}
	}
}

// removeProxy respond to delete proxy request
func removeProxy(configPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		file := mux.Vars(r)["name"] + ".json"

		err := os.Remove(configPath + "/" + file)
		if err != nil {
			w.WriteHeader(http.StatusNoContent)
			w.Write([]byte(err.Error()))
			return
		}
	}
}

// checkJSONAndRegister validate proxy configuration
func checkJSONAndRegister(rawData []byte, filename string, configPath string) (successful bool) {
	var cfg infrared.ProxyConfig
	err := json.Unmarshal(rawData, &cfg)
	if err != nil {
		log.Println(err)
		return false
	}

	if len(cfg.DomainNames) < 1 || cfg.ProxyTo == "" {
		return false
	}

	path := configPath + "/" + filename
	temppath := path + ".temp"

	err = os.WriteFile(temppath, rawData, 0644)
	if err != nil {
		log.Println(err)
		return false
	}

	err = os.Rename(temppath, path)
	if err != nil {
		return false
	}

	return true
}
