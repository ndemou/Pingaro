package monitor

type GroupID uint8

func (id GroupID) Index() int {
	return int(id)
}
