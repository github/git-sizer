package sizes

import ()

type pending interface {
	Run(*SizeScanner, *ToDoList) error
}

// A LIFO stack of `pending`s.
type ToDoList struct {
	list []pending
}

func (t *ToDoList) Push(pending pending) {
	t.list = append(t.list, pending)
}

func (t *ToDoList) PushAll(subtasks ToDoList) {
	t.list = append(t.list, subtasks.list...)
}

func (t *ToDoList) Pop() (pending, bool) {
	if len(t.list) == 0 {
		return nil, false
	}
	ret := t.list[len(t.list)-1]
	t.list[len(t.list)-1] = nil
	t.list = t.list[0 : len(t.list)-1]
	return ret, true
}

func (t *ToDoList) Run(scanner *SizeScanner, toDo *ToDoList) error {
	for {
		p, ok := t.Pop()
		if !ok {
			return nil
		}
		err := p.Run(scanner, toDo)
		if err != nil {
			return err
		}
	}
}
