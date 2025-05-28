package funpay

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/PuerkitoBio/goquery"
)

// type Chat struct {
// 	ID          int64
// 	UserName    string
// 	Link        string
// 	LastMessage int64
// 	isRead      bool
// 	MessageList []Message
// 	NewMessage  []Message
// }
//
// type Message struct {
// 	ID       string
// 	Author   string
// 	DateTime string
// 	Text     string
// }

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
	rs, err := ds.Html()
	cleaned := CleanHTML(html)
	clar := CleanHTML(rs)
	fmt.Println(clar)
	// fmt.Println(cleaned)
	fp.NewMessages(cleaned, link)
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
			if err != nil {
				fmt.Println("Некорректный id:", err)
				return err
			}
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

	idInt, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		fmt.Println("Некорректный id:", err)
		return
	}

	if err != nil {
		fmt.Println("no file data", err)
		fp.AddChat(idInt, chat)
	} else {

		found := false

		for _, c := range chats {
			if c.ID == idInt {
				found = true
				break
			}
		}

		if !found {
			idInt, err := strconv.ParseInt(id, 10, 64)
			if err != nil {
				fmt.Println("Некорректный id:", err)
				return
			}
			fp.AddChat(idInt, chat)
		}
	}
}

func (fp *Funpay) NewMessages(html, link string) {
	var chat Chat
	q, _ := url.Parse(link)
	id := q.Query().Get("node")
	msgs, err := fp.ParseAllMessagesFromChat(html)
	msgID, err := strconv.ParseInt(msgs[len(msgs)-1].ID, 10, 64)
	IntID, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		log.Println(err)
	}
	chat.ID = IntID
	chat.LastMessage = msgID
	chat.MessageList = msgs
	fp.GetNewMessages(chat)
	fp.UpdateChat(chat)
}

func (fp *Funpay) GetNewMessages(chat Chat) {
	// Читаем файл chats.json
	data, err := os.ReadFile("chats.json")
	if err != nil {
		fmt.Println("Error read file:", err)
		return
	}
	var item Chat
	var index int
	found := false
	err = json.Unmarshal(data, &chats)

	for i, it := range chats {
		if it.ID == chat.ID {
			item = it
			index = i
			break
		}
	}

	for i, msg := range chat.MessageList {
		idD, _ := strconv.ParseInt(msg.ID, 10, 64)
		if item.LastMessage == idD {
			fmt.Println("nead id")
			found = true
		}
		if found {
			fmt.Println("nead id", i)
			chats[index].NewMessage = append(chats[index].NewMessage, msg)
		}
	}
	fmt.Println(chats[index].NewMessage)
}

func (fp *Funpay) UpdateChat(chat Chat) error {
	const op = "Funpay.UpdateChat"

	data, err := os.ReadFile("chats.json")
	if err != nil {
		fmt.Println("Error read file:", err)
		return fmt.Errorf("%s: %w", op, err)
	}

	err = json.Unmarshal(data, &chats)
	if err != nil {
		fmt.Println("no file data", err)
		fp.AddChat(chat.ID, chat)
	}
	// fmt.Println("JSON: ", chats, "\nКонец JSON")
	for i, readChat := range chats {
		if readChat.ID == chat.ID {
			chats[i].LastMessage = chat.LastMessage
			chats[i].MessageList = append(chats[i].MessageList, chat.MessageList...)
		}
	}

	file, err := os.Create("chats.json")
	if err != nil {
		fmt.Println("Ошибка при создании файла:", err)
		return fmt.Errorf("%s: %w", op, err)
	}
	defer file.Close()

	// Кодируем слайс в JSON и записываем в файл с отступами для читаемости
	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")                 // Отступы для удобного чтения
	cleanedChats := removeDuplicateChats(chats) // очищаем повторения
	if err := encoder.Encode(cleanedChats); err != nil {
		fmt.Println("Ошибка при кодировании JSON:", err)
		return fmt.Errorf("%s: %w", op, err)
	}

	fmt.Println("Данные успешно сохранены в chats.json")
	return nil
}

func (fp *Funpay) AddChat(id int64, chat Chat) error {
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
	// fmt.Println(chats)

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

func (fp *Funpay) ParseChatName(funpayChatID string) (string, error) {
	link := fp.baseURL + "/chat/?node=" + funpayChatID
	doc, err := fp.RequestHTML(context.Background(), link)
	if err != nil {
		return "", err
	}
	// Парсим HTML через goquery

	// Ищем нужный div
	selector := fmt.Sprintf(`div.chat.chat-float[data-id="%s"]`, funpayChatID)
	chatDiv := doc.Find(selector)
	if chatDiv.Length() == 0 {
		return "", fmt.Errorf("div not found")
	}

	chatName, exists := chatDiv.Attr("data-name")
	if !exists {
		return "", fmt.Errorf("data-name attribute not found")
	}

	return chatName, nil
}

func (fp *Funpay) SendMessage(chatID, message string) error {
	chatName, err := fp.ParseChatName(chatID)
	if err != nil {
		log.Println(err)
	}
	postURL := fp.baseURL + "/runner/"
	requestData := map[string]any{
		"node":         chatName,
		"last_message": -1,
		"content":      message,
	}
	r := map[string]any{
		"action": "chat_message",
		"data":   requestData,
	}
	postBody, _ := json.Marshal(r)
	body := url.Values{}
	body.Set("objects", "[]")
	body.Set("request", string(postBody))
	body.Set("csrf_token", fp.CSRFToken())
	resp, err := fp.Request(context.Background(), postURL, RequestWithMethod(http.MethodPost), RequestWithBody(bytes.NewBufferString(body.Encode())), RequestWithHeaders(map[string]string{
		"content-type":     "application/x-www-form-urlencoded; charset=UTF-8",
		"accept":           "*/*",
		"x-requested-with": "XMLHttpRequest",
	}),
	)
	if err != nil {
		return err
	}
	html, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println(string(html))
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

func removeDuplicateMessages(messages []Message) []Message {
	seen := make(map[string]bool)
	result := make([]Message, 0, len(messages))
	for _, msg := range messages {
		if !seen[msg.ID] {
			seen[msg.ID] = true
			result = append(result, msg)
		}
	}
	return result
}

func removeDuplicateChats(chats []Chat) []Chat {
	seen := make(map[int64]bool)
	result := make([]Chat, 0, len(chats))
	for _, chat := range chats {
		if !seen[chat.ID] {
			seen[chat.ID] = true
			// Чистим MessageList
			chat.MessageList = removeDuplicateMessages(chat.MessageList)
			result = append(result, chat)
		}
	}
	return result
}
