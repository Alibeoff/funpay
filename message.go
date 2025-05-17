package funpay

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

type Chat struct {
	ID          int64
	UserName    string
	Link        string
	LastMessage int64
	isRead      bool
	MessageList []Message
	NewMessage  []Message
}

type Message struct {
	ID       string
	Author   string
	DateTime string
	Text     string
}

// id -yes name -yes link yes lastMessage - yes isread - nil messagelist new message - nil
var chats []Chat

// Method to do GET response for getting HTML page code
func (fp *Funpay) GetAllMessages(ctx context.Context) error {

	const op = "Funpay.GetAllMessages"
	resp, err := fp.RequestHTML(ctx, fp.baseURL+"/chat/")
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	err = fp.GetMessageInfo(resp)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	return nil
}

func (fp *Funpay) GetAllUnreadMessagesHTML(link string) error {
	const op = "Funpay.GetAllUnreadMessages"
	resp, err := fp.RequestHTML(context.Background(), link)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	ds := resp.Find(".chat-message-list")
	html, err := ds.Html()
	cleaned := CleanHTML(html)
	// fmt.Println(cleaned)
	fp.NewMessages(cleaned)
	//TODO: Прописать логику консольлога нового собщения только новых!!!!
	/*
		Для этого разделяем весь файл на список формируем слайс структуры сообщение.чат по которой проходимся
		в поиске прочитанного сообщения. после чего устанавливаем значение показа сообщений на тру и показваем все сообщения в консоли!!!
	*/
	return nil
}

// Funpay method for get all Messages from html
func (fp *Funpay) GetMessageInfo(doc *goquery.Document) error {
	const op = "Funpay.GetMessagesInfo"
	ds := doc.Find(".contact-list")
	html, err := ds.Html()
	// fmt.Println(html)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	items, err := ParseContacts(html)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	for _, item := range items {
		nmsg := "no found"
		if !item.isRead {
			fp.GetAllUnreadMessagesHTML(item.Link)
			nmsg = "found"
		}
		fmt.Printf("link: %s, nickname: %s, new messages: %s\n", item.Link, item.UserName, nmsg)
		fp.ParseChat(item)
	}
	return nil
}

func (fp *Funpay) ParseChat(chat Chat) {
	re := regexp.MustCompile("[0-9]+")
	nums := re.FindAllString(chat.Link, -1)
	id := nums[0]
	// Читаем файл chats.json
	data, err := os.ReadFile("chats.json")
	if err != nil {
		fmt.Println("Error read file:", err)
		return
	}

	err = json.Unmarshal(data, &chats)
	if err != nil {
		fmt.Println("no file data", err)
		fp.AddChat(id, chat)
	} else {
		idInt, err := strconv.ParseInt(id, 10, 64)
		if err != nil {
			fmt.Println("Некорректный id:", err)
			return
		}

		found := false

		for _, c := range chats {
			if c.ID == idInt {
				found = true
				break
			}
		}

		if !found {
			fp.AddChat(id, chat)
		}
	}
}

func (fp *Funpay) NewMessages(html string) {
	msgs, err := fp.ParseAllMessagesFromChat(html)
	if err != nil {
		log.Println(err)
	}
	fmt.Println(msgs)
}

func (fp *Funpay) AddChat(id string, chat Chat) error {
	const op = "Funpay.AddChat"
	doc, err := fp.RequestHTML(context.Background(), chat.Link)
	if err != nil {
		return fmt.Errorf("%s: %w", op, err)
	}
	// Находим все div с классом chat-msg-item
	messages := doc.Find("div.chat-msg-item")

	messages.Each(func(i int, s *goquery.Selection) {
		msg := Message{}

		// Извлекаем ID из атрибута id, например "message-3229540561"
		if id, exists := s.Attr("id"); exists {
			id = strings.ReplaceAll(id, "message-", "")
			msg.ID = id
		}

		// Автор: ищем ссылку с классом chat-msg-author-link внутри media-user-name
		author := s.Find(".media-user-name a.chat-msg-author-link").First().Text()
		msg.Author = strings.TrimSpace(author)

		// Дата и время: div с классом chat-msg-date
		dateTime := s.Find(".chat-msg-date").First().Text()
		msg.DateTime = strings.TrimSpace(dateTime)

		// Текст сообщения: div с классом chat-msg-text
		text := s.Find(".chat-msg-text").First().Text()
		msg.Text = strings.TrimSpace(text)

		// Добавляем сообщение в список
		chat.MessageList = append(chat.MessageList, msg)
		MsgID, err := strconv.Atoi(msg.ID)
		id, err := strconv.Atoi(id)
		if err != nil {
			return
		}
		chat.LastMessage = int64(MsgID)
		chat.ID = int64(id)
		// Если это последний элемент, выводим ID
		if i == messages.Length()-1 {
			fmt.Println("Last Message: ", msg.ID)
		}
	})

	chats = append(chats, chat)
	fmt.Println(chats)

	file, err := os.Create("chats.json")
	if err != nil {
		fmt.Println("Ошибка при создании файла:", err)
		return fmt.Errorf("%s: %w", op, err)
	}
	defer file.Close()

	// Кодируем слайс в JSON и записываем в файл с отступами для читаемости
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ") // Отступы для удобного чтения

	if err := encoder.Encode(chats); err != nil {
		fmt.Println("Ошибка при кодировании JSON:", err)
		return fmt.Errorf("%s: %w", op, err)
	}

	fmt.Println("Данные успешно сохранены в chats.json")
	return nil
}

func (fp *Funpay) ParseAllMessagesFromChat(html string) ([]Message, error) {
	var msgs []Message
	const op = "Funpay.ParseAllMessagesFromChat"
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return msgs, err
	}
	messages := doc.Find("div.chat-msg-item")
	messages.Each(func(i int, s *goquery.Selection) {

		msg := Message{}
		// Извлекаем ID из атрибута id, например "message-3229540561"
		if id, exists := s.Attr("id"); exists {
			id = strings.ReplaceAll(id, "message-", "")
			msg.ID = id
		}

		// Автор: ищем ссылку с классом chat-msg-author-link внутри media-user-name
		author := s.Find(".media-user-name a.chat-msg-author-link").First().Text()
		msg.Author = strings.TrimSpace(author)

		// Дата и время: div с классом chat-msg-date
		dateTime := s.Find(".chat-msg-date").First().Text()
		msg.DateTime = strings.TrimSpace(dateTime)

		// Текст сообщения: div с классом chat-msg-text
		text := s.Find(".chat-msg-text").First().Text()
		msg.Text = strings.TrimSpace(text)
		msgs = append(msgs, msg)
	})
	return msgs, nil
}
func FindMessageByID(html, msgID string) (*Message, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	selector := fmt.Sprintf("div.chat-msg-item#message-%s", msgID)
	s := doc.Find(selector)
	if s.Length() == 0 {
		return nil, fmt.Errorf("message with id %s not found", msgID)
	}

	author := s.Find(".chat-msg-author-link").Text()
	date := s.Find(".chat-msg-date").Text()
	text := s.Find(".chat-msg-text").Text()

	return &Message{
		ID:       msgID,
		Author:   author,
		DateTime: date,
		Text:     text,
	}, nil
}

// Function for getting all chats and not read messages
func ParseContacts(html string) ([]Chat, error) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(html))
	if err != nil {
		return nil, err
	}

	var contacts []Chat
	doc.Find("a.contact-item").Each(func(i int, s *goquery.Selection) {
		link, exists := s.Attr("href")
		if !exists {
			return
		}
		nickname := s.Find(".media-user-name").Text()

		// Если есть класс "unread" - значит сообщение непрочитано, IsRead = false
		isRead := !s.HasClass("unread")

		contacts = append(contacts, Chat{
			Link:     link,
			UserName: nickname,
			isRead:   isRead,
		})
	})

	return contacts, nil
}

func CleanHTML(html string) string {
	html = strings.ReplaceAll(html, "\n", " ")
	// html = strings.ReplaceAll(html, "\t", " ")

	reSpaces := regexp.MustCompile(`\s+`)
	html = reSpaces.ReplaceAllString(html, " ")

	html = strings.TrimSpace(html)

	return html
}
