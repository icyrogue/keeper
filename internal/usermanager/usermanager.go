package usermanager

import (
	"context"
	"fmt"
	"net/mail"

	//	"crypto/tls"
	"errors"
	"log"

	"github.com/golang-jwt/jwt"
	"github.com/jackc/pgx/v5/pgxpool"
)

type userManager struct {
	db                  *pgxpool.Pool
	Options             *Options
	NotificationManager NotificationManager
}

type Options struct {
	SecretKey string
}

type NotificationManager interface {
	Send(addr string, body []byte) error
}

func New(databasePool *pgxpool.Pool) *userManager {
	return &userManager{db: databasePool}
}

func (u *userManager) Init() error {
	_, err := u.db.Exec(context.Background(), `CREATE TABLE IF NOT EXISTS users(email TEXT, lists TEXT[])`)
	if err != nil {
		return err
	}

	return nil
}

// Check: checks validity of token
func (u *userManager) Check(ctx context.Context, id, tokenString string) error {
	k := func(token *jwt.Token) (interface{}, error) {
		return []byte(u.Options.SecretKey), nil
	}
	var user map[string]interface{}
	tokenToCheck, err := jwt.Parse(tokenString, k)
	if err != nil {
		log.Println(err.Error())
		return errors.New("error while verifying access")
	}
	if claims, ok := tokenToCheck.Claims.(jwt.MapClaims); ok && tokenToCheck.Valid {
		user = claims
		var count string
		log.Println("checking token for", user["email"], id)
		err = u.db.QueryRow(ctx, "SELECT email from users WHERE email = $1 AND $2 = ANY (lists)", user["email"], id).Scan(&count)
		if err != nil {
			log.Println(err.Error())
			return errors.New("error while verifying access")
		}
		return nil
	}
	return errors.New("error while verifying access")
}

// New user: links user email to new token, returns jwt token
func (u *userManager) NewUser(ctx context.Context, email string) (string, error) {
	if _, err := mail.ParseAddress(email); err != nil {
		return "", err
	}
	_, err := u.db.Exec(ctx, "INSERT INTO users (email, lists) VALUES($1, ARRAY []::TEXT[])", email)
	if err != nil {
		return email, errors.New("erros while creating a new user with this email")
	}

	token := jwt.New(jwt.SigningMethodHS256)
	claims := token.Claims.(jwt.MapClaims)
	claims["authorized"] = true
	claims["email"] = email

	tokenString, err := token.SignedString([]byte(u.Options.SecretKey))
	if err != nil {
		return "", err
	}
	err = u.NotificationManager.Send(email, []byte("<html><body>"+"Ваш пароль: "+tokenString+"</body></html>"))
	if err != nil {
		return "", err
	}
	return tokenString, nil
}

// Add to user lists: adds list ID to to user's available to user
func (u *userManager) AddToUserLists(ctx context.Context, id, email string) error {
	_, err := u.db.Exec(ctx, `UPDATE users SET lists = array_append(lists, $1) where email = $2`, id, email)
	if err != nil {
		return err
	}
	return nil
}

// GetUser: Returns email of a user
func (u *userManager) GetUser(ctx context.Context, id string) (string, error) {
	rows, err := u.db.Query(ctx, "SELECT email FROM users WHERE $1 = ANY (lists) LIMIT 1", id)
	if err != nil {
		return "", nil
	}
	var email string
	for rows.Next() {
		err = rows.Scan(&email)
		if err != nil {
			log.Println(err.Error())
			return "", err
		}
	}
	return email, nil
}

// GetUserIDs: returns all of the user available IDs
func (u *userManager) GetUserIDs(ctx context.Context, email string) (ids []string, err error) {
	err = u.db.QueryRow(ctx, "SELECT lists FROM users WHERE email = $1", email).Scan(&ids)
	if err != nil {
		return nil, err
	}
	if ids == nil {
		return nil, errors.New("user has no lists available")
	}
	return ids, err
}

// CheckWithEmail checks jwt token and returns email of a user from token claims. Needed for list generation
func (u *userManager) CheckWithEmail(ctx context.Context, tokenString string) (string, error) {
	k := func(token *jwt.Token) (interface{}, error) {
		return []byte(u.Options.SecretKey), nil
	}
	var user map[string]interface{}
	tokenToCheck, err := jwt.Parse(tokenString, k)
	if err != nil {
		log.Println(err.Error())
		return "", errors.New("error while verifying user")
	}
	if claims, ok := tokenToCheck.Claims.(jwt.MapClaims); ok && tokenToCheck.Valid {
		user = claims
		return fmt.Sprint(user["email"]), nil
	}
	return "", errors.New("error while verifying user")
}
