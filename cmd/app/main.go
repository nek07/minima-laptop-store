package main

import (
	"context"
	_ "context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	_ "log"
	"math/rand"
	"net/http"
	"net/smtp"
	"net/url"
	"os"
	"time"

	"html/template" //end damir

	"store/internal/database"
	"store/internal/models"
	"store/service/chat"
	"strconv"

	"github.com/golang-jwt/jwt"
	"github.com/google/uuid"
	_ "github.com/joho/godotenv/autoload"

	//Damir
	_ "github.com/eminetto/mongo-migrate"
	"go.mongodb.org/mongo-driver/bson"
	_ "go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"golang.org/x/crypto/bcrypt"
	"golang.org/x/time/rate"

	"github.com/gorilla/mux"
	"github.com/sirupsen/logrus"
	"github.com/sirupsen/logrus/hooks/writer"
	// "sync"
)

type Data1 struct {
	DocumentCount int64 `json:"DocumentCount"`
	Laptops       []models.Laptop
}

const uri = "mongodb+srv://damir:CNW6CNosCC9VFPoG@cluster0.qazvzjk.mongodb.net/?retryWrites=true&w=majority&appName=Cluster0"

var log = logrus.New()
var limiter = rate.NewLimiter(1, 3) // Rate limit of 1 request per second with a burst of 3 requests

func init() {
	// Set log formatter
	log.SetFormatter(&logrus.JSONFormatter{})

	// Create a log file or open existing
	logfile, err := os.OpenFile("logs.txt", os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}

	// Set up the File hook for the logrus logger
	log.AddHook(&writer.Hook{
		Writer: logfile,
		LogLevels: []logrus.Level{
			logrus.InfoLevel,
			logrus.WarnLevel,
			logrus.ErrorLevel,
			logrus.FatalLevel,
			logrus.PanicLevel,
		},
	})

	// Close the log file when the application exits
	defer logfile.Close()
}

func handleRoutes() {
	r := mux.NewRouter()
	r.PathPrefix("/static/").Handler(http.StripPrefix("/static/", http.FileServer(http.Dir("public"))))

	r.HandleFunc("/", homeHandler)
	r.HandleFunc("/", homeHandler) //correct header
	r.HandleFunc("/registration/verify", confirmVerificationCodeHandler)
	r.HandleFunc("/registration/verification", verificationPageHandler)
	r.HandleFunc("/registration", registrationPageHandler)
	r.HandleFunc("/logins", loginPageHandler)
	r.HandleFunc("/login", loginHandler)
	r.HandleFunc("/register", registerHandler)

	r.HandleFunc("/admin/submitNewsletter", newsletterHandler)
	r.HandleFunc("/error", rateLimitedHandler(errorPageHandler))

	r.HandleFunc("/updateUser", verifyToken(rateLimitedHandler(handleUpdateUser)))
	r.HandleFunc("/deleteUser", verifyToken(rateLimitedHandler(handleDeleteUser)))
	r.HandleFunc("/getAllUsers", verifyToken(rateLimitedHandler(handleGetAllUsers)))
	r.HandleFunc("/admin", verifyRole(verifyToken(rateLimitedHandler(handleAdmin))))
	r.HandleFunc("/products", verifyToken(productsPageHandler))
	r.HandleFunc("/productsTest", productsPageHandler)

	r.HandleFunc("/product/{id}", handleConcreteProduct)
	r.HandleFunc("/product/{id}/add-comment", verifyToken(rateLimitedHandler(addCommentHandler)))
	r.HandleFunc("/product/{id}/get-comments", verifyToken(rateLimitedHandler(getCommentHandler)))
	r.HandleFunc("/admin/delete/{id}", verifyRole(verifyToken(rateLimitedHandler(handleDeleteProduct))))
	r.HandleFunc("/admin/edit/{id}", verifyRole(verifyToken(rateLimitedHandler(handleEditProduct))))
	r.HandleFunc("/admin/add", verifyRole(verifyToken(rateLimitedHandler(addProdHandle))))
	r.HandleFunc("/basket", verifyToken(rateLimitedHandler(basketHandler)))
	r.HandleFunc("/product/{id}/addToBasket", verifyToken(rateLimitedHandler(addToCartHandler)))
	r.HandleFunc("/view-cart", verifyToken(rateLimitedHandler(viewCartHandler)))
	r.HandleFunc("/remove-from-cart/{id}", verifyToken(rateLimitedHandler(removeFromCartHandler)))
	r.HandleFunc("/account", verifyToken(rateLimitedHandler(accountHandler)))
	r.HandleFunc("/account/{id}/edit", verifyToken(rateLimitedHandler(editAccountHandler)))
	r.HandleFunc("/account/logout", verifyToken(rateLimitedHandler(logoutHandler)))
	r.HandleFunc("/delete_chat", deleteChatHandler)
	r.HandleFunc("/support", chatHandler)
	room := chat.NewRoom("maroom")
	go room.Run()
	r.HandleFunc("/room", room.HandleRoom)
	port := 10000
	fmt.Printf("Server is running on http://localhost:%d\n", port)
	// Использование маршрутизатора вместо http.ListenAndServe
	err := http.ListenAndServe(fmt.Sprintf(":%d", port), r)
	if err != nil {
		log.WithError(err).Error("Error starting server")
	}
}
func generateJWT(password string, username string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"password": password,
		"username": username,
		"exp":      time.Now().Add(time.Hour * 1).Unix(), // Token expires in 1 hour
	})

	// Sign the token with the secret key
	tokenString, err := token.SignedString(secretKey)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}
func verifyUser(r *http.Request) bool {
	cookie, err := r.Cookie("token")
	if err != nil {
		return false
	}
	token := cookie.Value
	user, err := database.FindUserByToken(token)
	if err != nil {
		return false
	}
	if user == nil {
		return false
	}
	return true
}
func main() {
	database.ConnectDB()
	handleRoutes()

}
func homeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		error404PageHandler(w, r)
		return
	}

	if r.Method == http.MethodGet {
		tmpl, err := template.ParseFiles("public/index.html")
		if err != nil {
			fmt.Println("Error parsing HTML template:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}

		// Execute the template with the list of ViewData items
		tmpl.Execute(w, verifyUser(r))
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
func registrationPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/registration" {
		error404PageHandler(w, r)
		return
	}

	if r.Method == http.MethodGet {
		http.ServeFile(w, r, "public/registration.html")
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

type TemplateData struct {
	Email  string
	ChatID string
}

func chatHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/support" {
		error404PageHandler(w, r)
		return
	}

	if r.Method == http.MethodGet {
		// Get the email from the request or from the session, depending on your implementation
		email := chat.GetEmailFromCookie(r) // Assuming you have a function to get email from cookie

		// Generate a new random chat ID
		chatID := uuid.New().String()

		// Create a new chat room with the generated chat ID
		room := chat.NewRoom(chatID)

		// Start the chat room
		go room.Run()

		// Create a data struct with the email and chat ID
		data := TemplateData{
			Email:  email,
			ChatID: chatID,
		}

		// Parse the HTML template
		tmpl, err := template.ParseFiles("public/chat.html")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		// Execute the template with the data
		err = tmpl.Execute(w, data)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func loginPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logins" {
		error404PageHandler(w, r)
		return
	}

	if r.Method == http.MethodGet {
		http.ServeFile(w, r, "public/login.html")
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
func verificationPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/registration/verification" {
		error404PageHandler(w, r)
		return
	}

	if r.Method == http.MethodGet {
		http.ServeFile(w, r, "public/verification.html")
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
func rateLimitedHandler(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !limiter.Allow() {
			// Exceeded request limit
			log.WithFields(logrus.Fields{
				"action":    "rate_limit_exceeded",
				"timestamp": time.Now().Format(time.RFC3339),
			}).Error("Rate limit exceeded")

			// Read the HTML template content
			htmlContent, err := ioutil.ReadFile("public/rateLimited.html")
			if err != nil {
				log.WithError(err).Error("Error reading HTML template")
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				return
			}

			// Set the content type to HTML
			w.Header().Set("Content-Type", "text/html; charset=utf-8")

			// Write the HTML content to the response
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write(htmlContent)
			return
		}
		next(w, r)
	}
}

// Damir
type ValidationErrors struct {
	NameError            string
	EmailError           string
	UsernameError        string
	PasswordError        string
	ConfirmPasswordError string
}

func productsPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/products" {
		error404PageHandler(w, r)
		return
	}
	// page := r.URL.Query().Get("page")
	brands := []string{r.URL.Query().Get("brand")}
	sortBy := r.URL.Query().Get("sort")
	minPrice, err := strconv.Atoi(r.URL.Query().Get("min"))
	maxPrice, err := strconv.Atoi(r.URL.Query().Get("max"))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))

	if page <= 0 {
		page = 1
	}

	if r.URL.Query().Get("min") == "" {
		minPrice = 0
	}
	if r.URL.Query().Get("max") == "" {
		maxPrice = 999999999
	}
	filter := bson.D{}
	db1 := database.Client.Database("go-assignment-2")
	collection1 := db1.Collection("products")
	// query to get all users
	cursor, err := collection1.Find(context.Background(), filter)
	if err != nil {
		fmt.Println(err)
	}
	defer cursor.Close(context.Background())

	// Iterate through the cursor and print each user

	// brands := []string{"Apple"}

	result, count, err := database.FindProductsWithFilters(brands, minPrice, maxPrice, sortBy, page)
	// Log product filtering
	log.WithFields(logrus.Fields{
		"action":    "filter_products",
		"brands":    brands,
		"minPrice":  minPrice,
		"maxPrice":  maxPrice,
		"sortBy":    sortBy,
		"page":      page,
		"timestamp": time.Now().Format(time.RFC3339),
	}).Info("User filtered products")
	if err != nil {
		log.Fatal(err)
	}
	data := Data1{
		Laptops:       result,
		DocumentCount: count,
	}
	// Render the HTML template with the fetched data
	tmpl, err := template.ParseFiles("public/products.html")
	if err != nil {
		fmt.Println("Error parsing HTML template:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}

	// Execute the template with the list of ViewData items
	tmpl.Execute(w, data)
}
func errorPageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		http.ServeFile(w, r, "public/error.html")
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
func error404PageHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		http.ServeFile(w, r, "public/error404.html")
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
} //end damir

// func handleGetUser(w http.ResponseWriter, r *http.Request) {
// 	// Get user ID from the request parameters

// 	id := r.FormValue("userId")
// 	foundUser, err := database.FindUserByID(id)
// 	if err != nil {
// 		fmt.Println("user not found")
// 		return
// 	}
// 	log.Printf("Get user result: %+v\n", foundUser)
// 	// Convert userID to int

// 	// Find user by ID (dummy data for illustration)

//		// Respond with user data in a JSON format
//		if foundUser != nil {
//			respondWithJSON(w, foundUser)
//		} else {
//			respondWithMessage(w, "User not found")
//		}
//	}
func handleUpdateUser(w http.ResponseWriter, r *http.Request) {

	userIDHex := r.FormValue("updateUserId")
	newUsername := r.FormValue("newUsername")
	var err error = database.UpdateUserUsernameByID(userIDHex, newUsername)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(" username was successfully updated")
	respondWithMessage(w, "updated ofigeno")
}
func handleDeleteUser(w http.ResponseWriter, r *http.Request) {

	userIDHex := r.FormValue("deleteUserId")
	var err error = database.DeleteUserByID(userIDHex)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("User was successfully deleted")
	respondWithMessage(w, "Udalen ofigeno")
}
func handleGetAllUsers(w http.ResponseWriter, r *http.Request) {
	// Get user ID from the request parameters

	foundUsers, err := database.GetAllUsers()
	if err != nil {
		fmt.Println("user not found")
		return
	}
	log.Printf("Get user result: %+v\n", foundUsers)
	if foundUsers != nil {
		respondWithJSON(w, foundUsers)
	} else {
		respondWithMessage(w, "Users not found")
	}
}
func respondWithMessage(w http.ResponseWriter, msg string) {
	// Respond with an error message in JSON di
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"message": msg})
}

func respondWithJSON(w http.ResponseWriter, data interface{}) {
	// Respond with user data in JSON format
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(data)
}
func handleAdmin(w http.ResponseWriter, r *http.Request) {
	// brands := []string{r.URL.Query().Get("brand")}
	// sortBy := r.URL.Query().Get("sort")
	// minPrice, err := strconv.Atoi(r.URL.Query().Get("min"))
	// maxPrice, err := strconv.Atoi(r.URL.Query().Get("max"))
	// page, _ := strconv.Atoi(r.URL.Query().Get("page"))

	// if page <= 0 {
	// 	page = 1
	// }

	// if r.URL.Query().Get("min") == "" {
	// 	minPrice = 0
	// }
	// if r.URL.Query().Get("max") == "" {
	// 	maxPrice = 999999999
	// }
	// result, count, _ := db.FindProductsWithFilters(brands, minPrice, maxPrice, sortBy, page)
	// fmt.Println(result)
	tmpl, err := template.ParseFiles("public/admin.html")
	if err != nil {
		fmt.Println("Error parsing HTML template:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	documents, err := database.GetUniqueChatIDDocuments()
	if err != nil {
		http.Error(w, "Failed to retrieve chat documents", http.StatusInternalServerError)
		return
	}
	var chats []ChatDocument
	for _, doc := range documents {
		chatID, ok1 := doc["chat_id"].(string)

		if ok1 {
			chats = append(chats, ChatDocument{
				ChatID: chatID,
			})
		}
	}

	data := Data{
		Chats: chats,
	}

	// Execute the template with the list of ViewData items
	tmpl.Execute(w, data)
}
func deleteChatHandler(w http.ResponseWriter, r *http.Request) {
	chatID := r.URL.Query().Get("chat_id")

	// MongoDB client initialization
	// clientOptions := options.Client().ApplyURI("mongodb://localhost:27017")
	// client, err := mongo.Connect(context.Background(), clientOptions)
	// if err != nil {
	// 	http.Error(w, "Failed to connect to the database", http.StatusInternalServerError)
	// 	return
	// }
	// defer client.Disconnect(context.Background())

	// Database and collection selection
	collection := database.Client.Database("go-assignment-2").Collection("Chats")

	// Delete the chat document from the database
	_, err := collection.DeleteOne(context.Background(), bson.M{"chat_id": chatID})
	if err != nil {
		http.Error(w, "Failed to delete chat", http.StatusInternalServerError)
		return
	}

	// Respond with a success message
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Chat deleted successfully"))
}

type ChatDocument struct {
	ChatID string
}

type Data struct {
	Chats []ChatDocument
}

func handleConcreteProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	laptopID := vars["id"]
	laptop, err := database.FindProductById(laptopID)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	tmpl, err := template.ParseFiles("public/concreateProduct.html")
	if err != nil {
		fmt.Println("Error parsing HTML template:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	tmpl.Execute(w, laptop)
}
func handleDeleteProduct(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodDelete {
		vars := mux.Vars(r)
		laptopID := vars["id"]
		err := database.DeleteProduct(laptopID)
		if err != nil {
			respondWithMessage(w, "Error with Delete Product")
		}
		respondWithMessage(w, "Successfully delete product with '"+laptopID+"' id")
	} else {
		error404PageHandler(w, r)
	}
}
func handleEditProduct(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	laptopID := vars["id"]

	if r.Method == http.MethodGet {
		laptop, err := database.FindProductById(laptopID)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		tmpl, err := template.ParseFiles("public/productEdit.html")
		if err != nil {
			fmt.Println("Error parsing HTML template:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		tmpl.Execute(w, laptop)
	} else if r.Method == http.MethodPut {
		id := r.FormValue("id")
		brand := r.FormValue("brand")
		model := r.FormValue("model")
		description := r.FormValue("description")
		price, err := strconv.Atoi(r.FormValue("price"))
		if err != nil {
			http.Error(w, "Invalid price", http.StatusBadRequest)
			return
		}

		// Обновление продукта в базе данных
		err = database.UpdateProductInDB(id, brand, model, description, price)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error updating product: %v", err), http.StatusInternalServerError)
			return
		}

		// Отправка успешного ответа в формате JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message": "Product updated successfully. Update page to see product"}`)
	} else {
		error404PageHandler(w, r)
		return
	}
}
func addProdHandle(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		brand := r.FormValue("brand")
		model := r.FormValue("model")
		description := r.FormValue("description")
		price, err := strconv.Atoi(r.FormValue("price"))
		if err != nil {
			http.Error(w, "Invalid price", http.StatusBadRequest)
			respondWithMessage(w, "Product is not added")
			return
		}
		err = database.AddProduct(brand, model, description, price)
		if err != nil {
			respondWithMessage(w, "Product is not added")
			return
		}
		respondWithMessage(w, "Product successfully added")
	} else {
		error404PageHandler(w, r)
		return
	}
}

// Secret key for JWT signing
var secretKey = []byte("SecretYouShouldHide")

func getData(email string, username string, password string, token string, role string) models.User {
	user := models.User{
		ID:          primitive.NewObjectID(),
		Email:       email,
		Username:    username,
		Password:    password,
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
		AccessToken: token,
		Role:        role,
	}
	return user
}

var verificationCode string
var newUser models.User

func registerHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/register" {
		error404PageHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Method == http.MethodPost {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			log.Println("Error parsing form:", err)
			return
		}
	}
	user, _ := database.FindUserByEmail(r.FormValue("email"))
	if user != nil {
		http.Redirect(w, r, "/logins", http.StatusSeeOther)
	}
	// Generate a verification code
	verificationCode = generateVerificationCode()

	// Send the verification code to the user's email
	err := sendVerificationCode(r.FormValue("email"), verificationCode)
	if err != nil {
		http.Error(w, "Error sending verification code", http.StatusInternalServerError)
		log.Println("Error sending verification code:", err)
		return
	}

	fmt.Println(r.FormValue("email"))

	// Populate newUser with form data
	newUser = getData(r.FormValue("email"), r.FormValue("username"), r.FormValue("password"), "", "USER")
	fmt.Println(newUser)

	// Hash the password before storing it
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newUser.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set the hashed password in newUser
	newUser.Password = string(hashedPassword)

	http.Redirect(w, r, "/registration/verification", http.StatusSeeOther)
}
func verifyRole(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the token from the request cookie
		cookie, err := r.Cookie("token")
		if err != nil {
			http.Redirect(w, r, "/error", http.StatusSeeOther)
			return
		}

		token := cookie.Value

		// Verify the token and retrieve user information
		user, err := database.FindUserByToken(token)

		if user.Role != "ADMIN" {
			http.Redirect(w, r, "/error", http.StatusSeeOther)
			return
		}
		// Store user information in the request context
		ctx := context.WithValue(r.Context(), "user", user)

		// Call the next handler in the chain with the updated request context
		next.ServeHTTP(w, r.WithContext(ctx))
	}

}
func verifyToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the token from the request cookie
		cookie, err := r.Cookie("token")
		if err != nil {
			http.Redirect(w, r, "/logins", http.StatusSeeOther)
			return
		}

		token := cookie.Value

		// Verify the token and retrieve user information
		user, err := database.FindUserByToken(token)
		if err != nil {
			http.Redirect(w, r, "/logins", http.StatusSeeOther)
			return
		}

		// Store user information in the request context
		ctx := context.WithValue(r.Context(), "user", user)

		// Call the next handler in the chain with the updated request context
		next.ServeHTTP(w, r.WithContext(ctx))
	}

}

func confirmVerificationCodeHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/registration/verify" {
		error404PageHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if r.Method == http.MethodPost {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "Error parsing form", http.StatusBadRequest)
			log.Println("Error parsing form:", err)
			return
		}
	}

	if verificationCode == r.FormValue("verificationCode") {
		token, err := generateJWT(r.FormValue("password"), r.FormValue("username"))
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		fmt.Println(token)
		newUser.AccessToken = token
		err = database.InsertData(newUser)
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Set JWT token as HTTP cookie
		cookie := http.Cookie{
			Name:     "token",
			Value:    token,
			HttpOnly: true,
			Expires:  time.Now().Add(time.Hour * 1), // Token expires in 1 hour
		}
		http.SetCookie(w, &cookie)
		w.WriteHeader(http.StatusCreated)
		fmt.Fprint(w, "User successfully registered")
	} else {
		// Handle case where verification code is incorrect
		http.Error(w, "Invalid verification code", http.StatusUnauthorized)
		return
	}
}
func loginHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/login" {
		error404PageHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		log.Println("Error parsing form:", err)
		return
	}

	user, err := database.FindUserByEmail(r.FormValue("email"))
	if err != nil {
		http.Error(w, "Unauthorized: Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Compare the stored hashed password with the input password
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(r.FormValue("password")))
	if err != nil {
		http.Error(w, "Unauthorized: Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Generate JWT token
	token, err := generateJWT(user.Password, user.Username)
	if err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set the token in a cookie
	newCookie1 := http.Cookie{
		Name:     "token",
		Value:    token,
		HttpOnly: true,
		Expires:  time.Now().Add(time.Hour * 1), // Token expires in 1 hour
	}
	http.SetCookie(w, &newCookie1)
	newCookie2 := http.Cookie{
		Name:     "email",
		Value:    user.Email,
		HttpOnly: true,
	}

	http.SetCookie(w, &newCookie2)

	// Update user's access token in the database
	if err := database.UpdateUserUsernameByEmail(user.Email, "access_token", token); err != nil {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Return a success response
	fmt.Print("Login success")
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func generateVerificationCode() string {
	rand.Seed(time.Now().UnixNano())
	code := rand.Intn(999999) + 100000
	return fmt.Sprintf("%05d", code)
}

func sendVerificationCode(email, code string) error {

	// Set up the authentication credentials for the SMTP server
	auth := smtp.PlainAuth("", os.Getenv("MAIL"), os.Getenv("SMTP_PASSWORD"), os.Getenv("SMTP_SERVER"))

	// Compose the email message
	to := []string{email}
	from := os.Getenv("MAIL")
	subject := "Verification Code"
	body := fmt.Sprintf("Your verification code is: %s", code)
	message := []byte("Subject: " + subject + "\r\n\r\n" + body)

	// Connect to the SMTP server and send the email
	err := smtp.SendMail(fmt.Sprintf("%s:%s", os.Getenv("SMTP_SERVER"), os.Getenv("SMTP_PORT")), auth, from, to, message)
	if err != nil {
		return fmt.Errorf("failed to send email: %v", err)
	}

	fmt.Printf("Verification code sent to %s\n", email)
	return nil
}
func sendMessageToAllEmails(subject string, info string) error {
	// Set up the authentication credentials for the SMTP server
	auth := smtp.PlainAuth("", os.Getenv("MAIL"), os.Getenv("SMTP_PASSWORD"), os.Getenv("SMTP_SERVER"))
	emails, err := database.GetUserEmails()
	if err != nil {
		return fmt.Errorf("failed to get user emails: %v", err)
	}

	// Compose the email message
	from := os.Getenv("MAIL")
	message := []byte("Subject: " + subject + "\r\n\r\n" + info)

	// Loop through each email and send the message
	for _, email := range emails {
		to := []string{email}
		err := smtp.SendMail(fmt.Sprintf("%s:%s", os.Getenv("SMTP_SERVER"), os.Getenv("SMTP_PORT")), auth, from, to, message)
		if err != nil {
			// Log or handle the error as needed
			fmt.Printf("Failed to send message to %s: %v\n", email, err)
		} else {
			fmt.Printf("Message sent to %s\n", email)
		}
	}

	return nil
}
func newsletterHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/admin/submitNewsletter" {
		error404PageHandler(w, r)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form", http.StatusBadRequest)
		log.Println("Error parsing form:", err)
		return
	}
	err := sendMessageToAllEmails(r.FormValue("subject"), r.FormValue("info"))
	if err != nil {
		fmt.Print(err)
	}
	fmt.Print("sended success")
}

// func generateJWT(password string, username string) (string, error) {
// 	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
// 		"password": password,
// 		"username": username,
// 		"exp":      time.Now().Add(time.Hour * 1).Unix(), // Token expires in 1 hour
// 	})

// 	// Sign the token with the secret key
// 	tokenString, err := token.SignedString(secretKey)
// 	if err != nil {
// 		return "", err
// 	}

// 	return tokenString, nil
// }

type ShoppingCart struct {
	Items []models.Laptop `json:"items"`
}

// Handler for adding items to the shopping
func addToCartHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve item details from request
	vars := mux.Vars(r)
	laptopID := vars["id"]
	newItem, _ := database.FindProductById(laptopID)

	// Retrieve existing shopping cart from cookie or create a new one
	cart := getCartFromCookie(r)

	// Add the new item to the shopping d
	cart.Items = append(cart.Items, newItem)

	// Save the updated shopping cart to cookie
	saveCartToCookie(w, cart)

	fmt.Fprintf(w, "Item added to cart successfully!")
}

// Handler for viewing items in the shopping cart
func viewCartHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve shopping cart from cookie
	cart := getCartFromCookie(r)

	// Convert shopping cart to JSON and send as response
	cartJSON, err := json.Marshal(cart)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(cartJSON)
}

// Function to retrieve shopping cart from cookie
func getCartFromCookie(r *http.Request) ShoppingCart {
	cartCookie, err := r.Cookie("shopping_cart")
	if err != nil {
		// If cookie doesn't exist, return an empty cart
		return ShoppingCart{Items: []models.Laptop{}}
	}

	var cart ShoppingCart
	cartJSON, _ := url.QueryUnescape(cartCookie.Value)
	json.Unmarshal([]byte(cartJSON), &cart)
	return cart
}

// Function to save shopping cart to cookie
func saveCartToCookie(w http.ResponseWriter, cart ShoppingCart) {
	cartJSON, _ := json.Marshal(cart)
	cartJSONEscaped := url.QueryEscape(string(cartJSON))
	http.SetCookie(w, &http.Cookie{
		Name:  "shopping_cart",
		Value: cartJSONEscaped,
		Path:  "/",
	})
}
func basketHandler(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("token")
	if err != nil {
		return
	}
	token := cookie.Value
	user, err := database.FindUserByToken(token)
	if err != nil {
		return
	}
	tmpl, err := template.ParseFiles("public/basket.html")
	if err != nil {
		fmt.Println("Error parsing HTML template:", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
	tmpl.Execute(w, user.ID.Hex())
}

// Handler for removing an item from the shopping cart
// Handler for removing items from the shopping cart
func removeFromCartHandler(w http.ResponseWriter, r *http.Request) {
	// Retrieve item ID from request parameters
	vars := mux.Vars(r)
	laptopID := vars["id"]

	// Retrieve existing shopping cart from cookie
	cart := getCartFromCookie(r)

	// Find and remove the item from the shopping cart
	for i, item := range cart.Items {
		if item.ID == laptopID {
			// Remove the item from the slice
			cart.Items = append(cart.Items[:i], cart.Items[i+1:]...)
			break
		}
	}

	// Save the updated shopping cart to cookie
	saveCartToCookie(w, cart)

	fmt.Fprintf(w, "Item removed from cart successfully!")
}

func getUsernameFromToken(tokenString string) (string, error) {
	// Парсим токен с помощью секретного ключа
	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		// Проверяем, что токен использует алгоритм подписи HMAC и возвращаем секретный ключ
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return secretKey, nil
	})
	if err != nil {
		return "", err
	}

	// Проверяем, что токен действителен
	if !token.Valid {
		return "", errors.New("invalid token")
	}

	// Извлекаем данные пользователя из токена
	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return "", errors.New("error parsing claims")
	}

	// Извлекаем username из claims
	username, ok := claims["username"].(string)
	if !ok {
		return "", errors.New("error getting username from claims")
	}

	return username, nil
}

func addCommentHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		text := r.URL.Query().Get("text")
		vars := mux.Vars(r)
		laptopID := vars["id"]
		cookie, err := r.Cookie("token")
		if err != nil {
			http.Redirect(w, r, "/logins", http.StatusSeeOther)
			return
		}

		token := cookie.Value
		if token == "" {
			// Токен отсутствует или недействителен, возвращаем ошибку
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Извлекаем username из токена
		username, err := getUsernameFromToken(token)
		if err != nil {
			// Ошибка при извлечении username, возвращаем ошибку
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		err = database.AddComment(username, text, laptopID)
	} else {
		error404PageHandler(w, r)
		return
	}
}
func getCommentHandler(w http.ResponseWriter, r *http.Request) {
	// Проверяем, что метод запроса GET
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Получаем параметры из URL
	params := mux.Vars(r)
	laptopID := params["id"]

	// Получаем комментарии для указанного ноутбука
	comments, err := database.GetCommentsByLaptop(laptopID)
	if err != nil {
		http.Error(w, "Failed to get comments", http.StatusInternalServerError)
		return
	}

	// Преобразуем комментарии в формат JSON
	jsonResponse, err := json.Marshal(comments)
	if err != nil {
		http.Error(w, "Failed to marshal comments", http.StatusInternalServerError)

		return
	}
	// Отправляем ответ с JSON данными
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(jsonResponse)
}

// func verifyUser(r *http.Request) bool {
// 	cookie, err := r.Cookie("token")
// 	if err != nil {
// 		return false
// 	}
// 	token := cookie.Value
// 	user, err := db.FindUserByToken(db.Client, token)
// 	if err != nil {
// 		return false
// 	}
// 	if user == nil {
// 		return false
// 	}
// 	return true
// }

func accountHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		cookie, err := r.Cookie("token")
		if err != nil {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		token := cookie.Value
		tmpl, err := template.ParseFiles("public/profile.html")
		if err != nil {
			fmt.Println("Error parsing HTML template:", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}
		user, err := database.FindUserByToken(token)
		if user.Role == "ADMIN" {
			handleAdmin(w, r)
			return
		}
		// Execute the template with the list of ViewData items
		tmpl.Execute(w, user)
	}
}
func editAccountHandler(w http.ResponseWriter, r *http.Request) {
	// Получаем идентификатор аккаунта из URL
	vars := mux.Vars(r)
	id := vars["id"]
	// Получаем данные из тела запроса
	err := r.ParseForm()
	if err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	// Извлекаем данные из формы
	username := r.Form.Get("username")
	email := r.Form.Get("email")

	err = database.UpdateAccount(id, username, email)
	if err != nil {
		http.Error(w, "Failed to update account", http.StatusInternalServerError)
		return
	}

	// Возвращаем успешный ответ
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Account updated successfully"))
	fmt.Println(id + "   id")
}
func logoutHandler(w http.ResponseWriter, r *http.Request) {
	// Удаляем токен из cookie путем установки пустого значения и истечения срока действия
	http.SetCookie(w, &http.Cookie{
		Name:     "token",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true, // Важно для безопасности, чтобы JavaScript не мог получить доступ к токену
	})

	// Отправляем успешный ответ
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Logged out successfully"))
}
