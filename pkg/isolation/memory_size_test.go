package isolation

import (
 	"os/exec"
	"testing"
	"fmt"
)

func TestMemorySize(t *testing.T) {
	memorysize := MemorySize{cgroupName: "M", memorySize:"512M"}
	
	cmd := exec.Command("sh","-c","sleep 1h")
	err := cmd.Start()
	if err != nil {
			panic(err)
	}
	
        memorysize.Isolate(cmd.Process.Pid)
	
	fmt.Printf(memorysize.cgroupName)

}
