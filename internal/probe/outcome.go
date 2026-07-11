package probe

type Outcome struct {
	request     Request
	rttMillis   int
	destination string
	status      string
	warning     string
}

func NewOutcome(req Request, rttMillis int, destination, status, warning string) Outcome {
	if destination == "" {
		destination = req.Target
	}
	return Outcome{
		request:     req,
		rttMillis:   rttMillis,
		destination: destination,
		status:      status,
		warning:     warning,
	}
}

func (o Outcome) Request() Request {
	return o.request
}

func (o Outcome) RTTMilliseconds() int {
	return o.rttMillis
}

func (o Outcome) Destination() string {
	return o.destination
}

func (o Outcome) Status() string {
	return o.status
}

func (o Outcome) Warning() string {
	return o.warning
}
