package sizes

import (
	"fmt"
	"io"
)

// A LIFO stack of `Oid`s.
type ToDoList struct {
	list []Oid
}

func (t *ToDoList) Length() int {
	return len(t.list)
}

func (t *ToDoList) Push(oid Oid) {
	t.list = append(t.list, oid)
}

func (t *ToDoList) Peek() Oid {
	return t.list[len(t.list)-1]
}

func (t *ToDoList) Drop() {
	t.list = t.list[0 : len(t.list)-1]
}

// For debugging.
func (t *ToDoList) Dump(w io.Writer) {
	fmt.Fprintf(w, "todo list has %d items\n", t.Length())
	for i, idString := range t.list {
		fmt.Fprintf(w, "%8d %s\n", i, idString)
	}
	fmt.Fprintf(w, "\n")
}
