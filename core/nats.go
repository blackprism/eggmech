package core

type Stream struct {
	Name string
	Subjects []string
}

var Streams = []Stream{
	{
		Name: "MESSAGE",
		Subjects: []string{
			"message.>",
		},
	},
}
