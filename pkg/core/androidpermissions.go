package core

import "strings"

// AndroidPermissionShortcut maps the permission names a flow uses to the
// Android permission strings `pm grant` and Appium's changePermissions expect.
//
// This lived in three drivers as three identical copies, which is how
// setPermissions came to work on some of them and not others: the Appium driver
// had the values but never the mapping, so a shortcut like `notifications` had
// nothing to resolve to. One copy, shared.
//
// An unrecognised name is returned unchanged, so a flow can name a raw
// android.permission.* string that this list does not enumerate.
func AndroidPermissionShortcut(shortcut string) []string {

	switch strings.ToLower(shortcut) {
	case "location":
		return []string{
			"android.permission.ACCESS_FINE_LOCATION",
			"android.permission.ACCESS_COARSE_LOCATION",
			"android.permission.ACCESS_BACKGROUND_LOCATION",
		}
	case "camera":
		return []string{"android.permission.CAMERA"}
	case "contacts":
		return []string{
			"android.permission.READ_CONTACTS",
			"android.permission.WRITE_CONTACTS",
			"android.permission.GET_ACCOUNTS",
		}
	case "phone":
		return []string{
			"android.permission.READ_PHONE_STATE",
			"android.permission.CALL_PHONE",
			"android.permission.READ_CALL_LOG",
			"android.permission.WRITE_CALL_LOG",
			"android.permission.USE_SIP",
			"android.permission.PROCESS_OUTGOING_CALLS",
		}
	case "microphone":
		return []string{"android.permission.RECORD_AUDIO"}
	case "bluetooth":
		return []string{
			"android.permission.BLUETOOTH_CONNECT",
			"android.permission.BLUETOOTH_SCAN",
			"android.permission.BLUETOOTH_ADVERTISE",
		}
	case "storage":
		return []string{
			"android.permission.READ_EXTERNAL_STORAGE",
			"android.permission.WRITE_EXTERNAL_STORAGE",
			"android.permission.READ_MEDIA_IMAGES",
			"android.permission.READ_MEDIA_VIDEO",
			"android.permission.READ_MEDIA_AUDIO",
		}
	case "notifications":
		return []string{"android.permission.POST_NOTIFICATIONS"}
	case "medialibrary":
		return []string{
			"android.permission.READ_MEDIA_IMAGES",
			"android.permission.READ_MEDIA_VIDEO",
			"android.permission.READ_MEDIA_AUDIO",
		}
	case "calendar":
		return []string{
			"android.permission.READ_CALENDAR",
			"android.permission.WRITE_CALENDAR",
		}
	case "sms":
		return []string{
			"android.permission.SEND_SMS",
			"android.permission.RECEIVE_SMS",
			"android.permission.READ_SMS",
			"android.permission.RECEIVE_WAP_PUSH",
			"android.permission.RECEIVE_MMS",
		}
	case "sensors", "activity_recognition":
		return []string{
			"android.permission.BODY_SENSORS",
			"android.permission.ACTIVITY_RECOGNITION",
		}
	default:
		// Assume it's a full Android permission name
		if strings.HasPrefix(shortcut, "android.permission.") {
			return []string{shortcut}
		}
		// Try adding the prefix
		return []string{"android.permission." + strings.ToUpper(shortcut)}
	}
}
