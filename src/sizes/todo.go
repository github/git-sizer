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

func (t *ToDoList) PushAll(subtasks ToDoList) {
	t.list = append(t.list, subtasks.list...)
}

func (t *ToDoList) Pop() pending {
	ret := t.list[len(t.list)-1]
	t.list[len(t.list)-1] = nil
	t.list = t.list[0 : len(t.list)-1]
	return ret
}
