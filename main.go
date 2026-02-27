package main

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"time"

	"NewAPI-Gateway/common"
	"NewAPI-Gateway/middleware"
	"NewAPI-Gateway/model"
	"NewAPI-Gateway/router"
	"NewAPI-Gateway/service"
	"embed"
	"log"
	"strconv"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-contrib/sessions/redis"
	"github.com/gin-gonic/gin"
)

//go:embed web/build
var buildFS embed.FS

//go:embed web/build/index.html
var indexPage []byte

func main() {
	common.SetupGinLog()
	common.SysLog("NewAPI Gateway " + common.Version + " started")
	if os.Getenv("GIN_MODE") != "debug" {
		gin.SetMode(gin.ReleaseMode)
	}
	// Initialize SQL Database
	err := model.InitDB()
	if err != nil {
		common.FatalLog(err)
	}
	defer func() {
		err := model.CloseDB()
		if err != nil {
			common.FatalLog(err)
		}
	}()

	// Initialize Redis
	err = common.InitRedisClient()
	if err != nil {
		common.FatalLog(err)
	}

	// Initialize options
	model.InitOptionMap()

	// Start cron jobs (sync & checkin)
	service.StartCronJobs()
	defer service.StopCronJobs()

	// Initialize HTTP server
	server := gin.Default()
	server.Use(middleware.CORS())

	// Initialize session store
	if common.RedisEnabled {
		opt := common.ParseRedisOption()
		store, _ := redis.NewStore(opt.MinIdleConns, opt.Network, opt.Addr, opt.Password, []byte(common.SessionSecret))
		server.Use(sessions.Sessions("session", store))
	} else {
		store := cookie.NewStore([]byte(common.SessionSecret))
		server.Use(sessions.Sessions("session", store))
	}

	router.SetRouter(server, buildFS, indexPage)

	// Get port configuration
	port := os.Getenv("PORT")
	if port == "" {
		port = strconv.Itoa(*common.Port)
	}

	// Check if HTTPS is enabled
	httpsEnabled := os.Getenv("HTTPS_ENABLED") == "true"
	httpsCertFile := os.Getenv("HTTPS_CERT_FILE")
	httpsKeyFile := os.Getenv("HTTPS_KEY_FILE")

	if httpsEnabled {
		// HTTPS mode
		common.SysLog("HTTPS mode enabled")

		// If certificate files are not specified, generate self-signed certificate
		if httpsCertFile == "" || httpsKeyFile == "" {
			common.SysLog("No certificate specified, generating self-signed certificate...")
			certFile, keyFile, err := generateSelfSignedCert()
			if err != nil {
				common.FatalLog("Failed to generate self-signed certificate: " + err.Error())
			}
			httpsCertFile = certFile
			httpsKeyFile = keyFile
			common.SysLog("Self-signed certificate generated: " + certFile)
		}

		// Run HTTPS server
		common.SysLog("Starting HTTPS server on :" + port)
		err = server.RunTLS(":"+port, httpsCertFile, httpsKeyFile)
		if err != nil {
			common.FatalLog(err)
		}
	} else {
		// HTTP mode
		common.SysLog("Starting HTTP server on :" + port)
		err = server.Run(":" + port)
		if err != nil {
			log.Println(err)
		}
	}
}

// generateSelfSignedCert generates a self-signed SSL certificate
func generateSelfSignedCert() (certFile, keyFile string, err error) {
	// Generate private key
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return "", "", err
	}

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"NewAPI-Gateway"},
			Country:      []string{"US"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour), // Valid for 1 year
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Generate certificate
	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return "", "", err
	}

	// Save certificate to file
	certOut, err := os.Create("server.crt")
	if err != nil {
		return "", "", err
	}
	defer certOut.Close()
	pem.Encode(certOut, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	// Save private key to file
	keyOut, err := os.Create("server.key")
	if err != nil {
		return "", "", err
	}
	defer keyOut.Close()
	pem.Encode(keyOut, &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	return "server.crt", "server.key", nil
}
