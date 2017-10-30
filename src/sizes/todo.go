package sizes

import ()

// A LIFO stack of `Oid`s.
type ToDoList struct {
	list []pending
}

func (t *ToDoList) Length() int {
	return len(t.list)
}

func (t *ToDoList) Push(pending pending) {
	t.list = append(t.list, pending)
}

func (t *ToDoList) Peek() pending {
	return t.list[len(t.list)-1]
}

func (t *ToDoList) Drop() {
	t.list[len(t.list)-1] = pending{}
	t.list = t.list[0 : len(t.list)-1]
}
