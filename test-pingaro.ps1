# Create a copy of pingaro.exe and run it
# (Will terminate the copy if it is already running)

$p=(Get-Process -Name "test-pingaro" -ErrorAction SilentlyContinue)
if ($p) {$p|stop-process; sleep 1}
cp .\pingaro.exe test-pingaro.exe
.\test-pingaro.exe