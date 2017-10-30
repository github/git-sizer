package sizes

import ()

type pending interface {
	Run(*SizeScanner) error
}

// A LIFO stack of `pending`s.
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

func (t *ToDoList) Run(scanner *SizeScanner) error {
	for t.Length() != 0 {
		p := t.Pop()

		err := p.Run(scanner)
		if err != nil {
			return err
		}
	}

	return nil
}
