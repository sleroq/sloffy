package misskey

import "time"

type Misskey struct {
	Token string
}

func New(token string) *Misskey {
	return &Misskey{Token: token}
}

type Emoji struct {
	Name string
	Url string
}

type User struct {
	Id string
	Name string
	Username string
	Host string
	AvatarUrl string
	AvatarBlurhash interface{} // FIXME
	AvatarColor interface{} // FIXME
	IsAdmin bool
	IsModerator bool
	IsBot bool
	IsCat bool
	Emojis []Emoji
	OnlineStatus string
}

type File struct {
	Id string
	CreatedAt time.Time
	Name string
	Type string
	Md5 string
	Size int
	IsSensitive bool
	Blurhash string
	Properties
	Url "string"
	ThumbnailUrl "string"
	Comment "string"
	FolderId "xxxxxxxxxx"
	Folder
	UserId "xxxxxxxxxx"
	User
}

type CreatedNote struct {
    Id string
    CreatedAt time.Time
    Text string
    Cw string
    UserId string
    User User
	ReplyId string
	RenoteId string
	Reply interface{} // FIXME
	Renote interface{} // FIXME
	IsHidden bool
	Visibility string
	Mentions []string
	VisibleUserIds []string
	FileIds []string
	Files []File
	Tags []string
	Poll interface{} // FIXME
	ChannelId string
	Channel interface{} // FIXME
	LocalOnly bool
	Emojis []Emoji
	Reactions interface{} // FIXME
	RenoteCount int
	RepliesCount int
	Uri string
	Url string
	MyReaction interface{} // FIXME
}

func (api Misskey) NewNote(text string) (CreatedNote, error) {

}
