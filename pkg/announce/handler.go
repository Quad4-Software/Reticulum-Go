package announce

type Handler interface {
	AspectFilter() []string
	ReceivedAnnounce(destHash []byte, identity interface{}, appData []byte, hops uint8) error
	ReceivePathResponses() bool
}
