package main

import "fyne.io/fyne/v2"

type xinLayout struct {
	size fyne.Size
}

func (f *xinLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	f.size = fyne.NewSize(float32(800), float32(400))
	return f.size
}

func (f *xinLayout) Layout(objects []fyne.CanvasObject, containerSize fyne.Size) {
	pos := fyne.NewPos(0, containerSize.Height-f.MinSize(objects).Height)
	for _, o := range objects {
		size := o.MinSize()
		o.Resize(f.size)
		o.Move(pos)

		pos = pos.Add(fyne.NewPos(size.Width, size.Height))
	}
}
