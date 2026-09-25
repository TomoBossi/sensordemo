package main

import "testing"

func TestSpaceStepModeOnAtStart(t *testing.T) {
	ss, _ := OpenMock("step_detector=1;orient=0,80,0")
	s := find("space").new(nil).(*space)
	s.Setup(ss)
	var f Frame
	f.Resize(40, 20)
	s.Draw(f.View(0, 0, 40, 20), ss, 0, 1.0/30)
	if !s.stepMode || ss.Get("step_detector") == nil {
		t.Fatal("step mode should be on and subscribed at start")
	}
	s.Key('p')
	s.Draw(f.View(0, 0, 40, 20), ss, 0, 1.0/30)
	if s.stepMode || ss.Get("step_detector") != nil {
		t.Fatal("p should turn it off and unsubscribe")
	}
}
