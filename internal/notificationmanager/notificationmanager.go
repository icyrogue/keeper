package notificationmanager

import (
	"context"
	"log"
	"sync"

	mail "github.com/xhit/go-simple-mail/v2"
)

type notificationManager struct {
	mtx         sync.RWMutex
	userManager UserManager
	server      *mail.SMTPServer
	client      *mail.SMTPClient

	Options *Options
}

type Options struct {
	KeeperMail       string
	KeeperMailPasswd string
	MailHost         string
}

type UserManager interface {
	GetUser(ctx context.Context, id string) (string, error)
}

func New(userManager UserManager, options *Options) *notificationManager {
	if options.KeeperMail == "" {
		return &notificationManager{}
	}
	server := mail.NewSMTPClient()
	server.Port = 587
	server.Username = options.KeeperMail
	server.Password = options.KeeperMailPasswd
	server.Host = options.MailHost
	server.KeepAlive = true
	server.Encryption = mail.EncryptionSTARTTLS

	client, err := server.Connect()
	if err != nil {
		log.Fatal(err.Error())
	}

	return &notificationManager{userManager: userManager, server: server, client: client}
}

// Notify: notifies a user with specific ID
func (n *notificationManager) Notify(ctx context.Context, id string, data []byte) error {
	n.mtx.Lock()
	defer n.mtx.Unlock()

	email, err := n.userManager.GetUser(ctx, id)
	if err != nil {
		return err
	}
	err = n.Send(email, data)
	if err != nil {
		return err
	}
	return nil
}

// Sends email using SMTP connection
func (n *notificationManager) Send(addr string, body []byte) error {
	log.Println("preparing to send email to", addr)
	email := mail.NewMSG()
	email.SetFrom("componentkeeper@gmail.com")
	email.AddTo(addr)
	email.SetBodyData(mail.TextHTML, body)
	if email.Error != nil {
		log.Fatal(email.Error)
	}

	err := email.Send(n.client)
	if err != nil {
		log.Println(err)
	} else {
		log.Println("Email Sent to", addr)
	}
	return nil
}
