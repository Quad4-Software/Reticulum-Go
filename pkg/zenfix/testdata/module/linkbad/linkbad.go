package linkbad

type Link struct{}

func (l *Link) SetEstablishedCallback(func(*Link)) {}
func (l *Link) Establish() error                   { return nil }
func (l *Link) Send([]byte) error                  { return nil }
func (l *Link) Request(string, []byte) error       { return nil }

func NewLink() *Link { return &Link{} }

func BadNewLinkLoop() {
	for {
		NewLink()
	}
}

func BadLinkRequestLoop(l *Link) {
	for {
		_ = l.Request("/ping", nil)
	}
}

func BadLinkSendNoCallback(l *Link) error {
	return l.Send([]byte("early"))
}

func BadNewLinkRepeat() (*Link, error) {
	a := NewLink()
	b := NewLink()
	_ = a
	return b, nil
}

func BadLinkSendLoop(l *Link) {
	for {
		_ = l.Send([]byte("x"))
	}
}
