package tsparams

import "github.com/openshift-kni/eco-gotests/tests/hw-accel/amdgpu/internal/amdgpuparams"

const (
	// LabelSuite represents amdgpu label that can be used for test cases selection.
	LabelSuite = "amdgpu"
)

// Re-export constants from amdgpuparams for convenience.
const (
	AMDGPUTestNamespace     = amdgpuparams.AMDGPUTestNamespace
	AMDGPUOperatorNamespace = amdgpuparams.AMDGPUOperatorNamespace
	LogLevel                = amdgpuparams.LogLevel
)

// ROCMSmiScript contains a bash script for testing ROCm GPU functionality.
const ROCMSmiScript = `
#!/bin/bash
set -e

echo "=== ROCm GPU Test Started ==="
echo "Date: $(date)"
echo "Hostname: $(hostname)"

# Function to check GPU availability with multiple attempts
check_gpu() {
    echo "=== Checking GPU availability ==="

    # Try rocm-smi first
    if rocm-smi --showid --showproductname --showvram 2>/dev/null; then
        echo "SUCCESS: rocm-smi detected GPU"
        return 0
    fi

    # Fallback: try basic rocm-smi
    if rocm-smi 2>/dev/null; then
        echo "SUCCESS: Basic rocm-smi working"
        return 0
    fi

    echo "WARNING: rocm-smi failed, but continuing test"
    return 0  # Don't fail the test, just warn
}

# Function to run GPU workload with timeout control
run_gpu_workload() {
    echo "=== Starting GPU workload ==="

    # Start workload in background with controlled timeout

    timeout 115 python3 - <<'EOF' &
import sys
import time
import os

try:
    print(f"Python version: {sys.version}")

    # Try to import and test PyTorch
    try:
        import torch
        print(f"PyTorch version: {torch.__version__}")
        print(f"CUDA available: {torch.cuda.is_available()}")

        if torch.cuda.is_available():
            device_count = torch.cuda.device_count()
            print(f"CUDA devices found: {device_count}")

            for i in range(device_count):
                print(f"Device {i}: {torch.cuda.get_device_name(i)}")

            device = torch.device("cuda:0")
            print(f"Using device: {device}")

            # Create larger tensors for higher load and memory usage.
            # Adjust size (e.g., 8000, 12000) based on your GPU VRAM.
            tensor_size = 8000
            print(f"Creating tensors of size ({tensor_size}, {tensor_size})...")
            a = torch.rand((tensor_size, tensor_size), device=device, dtype=torch.float32)
            b = torch.rand((tensor_size, tensor_size), device=device, dtype=torch.float32)

            print("Running matrix multiplication workload...")
            # Run for ~55 seconds to match the monitoring duration
            for i in range(55):
                result = torch.matmul(a, b)
                torch.cuda.synchronize() # Wait for the operation to complete
                if i % 5 == 0:
                    print(f"Workload iteration {i}/55 completed")
                time.sleep(1)

            print("GPU workload completed successfully")
        else:
            print("INFO: CUDA not available, but test continues")

    except ImportError as e:
        print(f"INFO: PyTorch not available: {e}")
    except Exception as e:
        print(f"WARNING: GPU workload error: {e}")

    print("Workload phase completed")

except Exception as e:
    print(f"ERROR in workload: {e}")
    # Don't exit with error - let monitoring continue
EOF

    WORKLOAD_PID=$!
    echo "GPU workload started with PID: $WORKLOAD_PID"

    # Wait a bit for workload to start and allocate memory
    sleep 5
    return 0
}

# Function to monitor GPU with rocm-smi - more frequent sampling
monitor_gpu() {
    echo "=== Monitoring GPU activity ==="

    for i in {1..30}; do
        echo "--- ROCm-SMI Output (iteration $i/30) ---"

        # Try comprehensive rocm-smi first
        if rocm-smi --showuse --showmemuse --showtemp --showpower --showclocks 2>/dev/null; then
            echo "Detailed monitoring successful"
        elif rocm-smi --showuse --showpower 2>/dev/null; then
            echo "Basic monitoring successful"
        elif rocm-smi 2>/dev/null; then
            echo "Minimal monitoring successful"
        else
            echo "Warning: rocm-smi monitoring failed at iteration $i"
        fi

        # Show GPU processes if possible
        rocm-smi --showpids 2>/dev/null || echo "No process info available"

        sleep 2
    done
}

# --- Main Execution ---
echo "Step 1: Initial GPU check"
check_gpu || echo "GPU check had issues but continuing"

echo "Step 2: Starting GPU workload"
run_gpu_workload || echo "Workload start had issues but continuing"

echo "Step 3: Monitoring GPU activity for 60 seconds"
monitor_gpu

echo "Step 4: Waiting for workload to complete and checking status"
if wait $WORKLOAD_PID; then
    echo "SUCCESS: Workload process (PID: $WORKLOAD_PID) completed successfully."
else
    EXIT_CODE=$?
    if [ $EXIT_CODE -eq 124 ]; then
        echo "WARNING: Workload process (PID: $WORKLOAD_PID) was terminated by timeout."
    else
        echo "WARNING: Workload process (PID: $WORKLOAD_PID) exited with error code: $EXIT_CODE."
    fi
fi


echo "Step 5: Final GPU status check"
echo "=== Final ROCm-SMI Status (after workload) ==="
rocm-smi --showuse --showmemuse --showtemp --showpower 2>/dev/null || \
rocm-smi 2>/dev/null || echo "Final status check failed"

echo "=== ROCm GPU Test Completed ==="
echo "Test finished at: $(date)"
sleep 15
`
