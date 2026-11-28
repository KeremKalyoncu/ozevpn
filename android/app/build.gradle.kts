import java.util.Properties

plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
    id("org.jetbrains.kotlin.plugin.compose")
}

val releaseKeystoreProps = Properties().apply {
    val envPath = System.getenv("ANDROID_KEYSTORE_PATH")
    val envStorePw = System.getenv("ANDROID_KEYSTORE_PASSWORD")
    val envAlias = System.getenv("ANDROID_KEY_ALIAS")
    val envKeyPw = System.getenv("ANDROID_KEY_PASSWORD")
    if (!envPath.isNullOrBlank() && !envStorePw.isNullOrBlank() && !envAlias.isNullOrBlank() && !envKeyPw.isNullOrBlank()) {
        setProperty("storeFile", envPath)
        setProperty("storePassword", envStorePw)
        setProperty("keyAlias", envAlias)
        setProperty("keyPassword", envKeyPw)
    } else {
        val localProps = file("${System.getProperty("user.home")}/.ozevpn-keys/keystore.properties")
        if (localProps.isFile) {
            localProps.inputStream().use { load(it) }
        }
    }
}
val hasReleaseKeystore = releaseKeystoreProps.getProperty("storeFile")?.let { file(it).isFile } == true

android {
    namespace = "com.keremkalyoncu.ozevpn"
    compileSdk = 36

    defaultConfig {
        applicationId = "com.keremkalyoncu.ozevpn"
        minSdk = 29
        targetSdk = 35
        versionCode = 620
        versionName = "6.2.0"
    }

    dependenciesInfo {
        // Strip Google Play proprietary encrypted dependency block for IzzyOnDroid / F-Droid compliance
        includeInApk = false
        includeInBundle = false
    }

    signingConfigs {
        if (hasReleaseKeystore) {
            create("release") {
                storeFile = file(releaseKeystoreProps.getProperty("storeFile"))
                storePassword = releaseKeystoreProps.getProperty("storePassword")
                keyAlias = releaseKeystoreProps.getProperty("keyAlias")
                keyPassword = releaseKeystoreProps.getProperty("keyPassword")
                enableV1Signing = true
                enableV2Signing = true
                enableV3Signing = true
            }
        }
    }

    splits {
        abi {
            isEnable = true
            reset()
            include("arm64-v8a", "x86_64")
            isUniversalApk = true
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            isShrinkResources = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )
            // Use permanent release keystore when configured (CI / maintainer), fallback to debug for forks
            signingConfig = if (hasReleaseKeystore) {
                signingConfigs.getByName("release")
            } else {
                signingConfigs.getByName("debug")
            }
        }
        debug {
            isMinifyEnabled = false
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    buildFeatures {
        compose = true
        buildConfig = true
    }

    packaging {
        jniLibs {
            // Required so Android extracts libsingbox.so into nativeLibraryDir with exec permission
            useLegacyPackaging = true
        }
        resources {
            excludes += "/META-INF/{AL2.0,LGPL2.1}"
        }
    }
}

dependencies {
    implementation(files("libs/hev-socks5-tunnel.aar"))

    val composeBom = platform("androidx.compose:compose-bom:2025.04.01")
    implementation(composeBom)
    implementation("androidx.core:core-ktx:1.15.0")
    implementation("androidx.activity:activity-compose:1.10.1")
    implementation("androidx.lifecycle:lifecycle-runtime-ktx:2.8.7")
    implementation("androidx.lifecycle:lifecycle-runtime-compose:2.8.7")
    implementation("androidx.compose.ui:ui")
    implementation("androidx.compose.ui:ui-graphics")
    implementation("androidx.compose.material3:material3")
    implementation("androidx.compose.material:material-icons-extended")
    implementation("org.jetbrains.kotlinx:kotlinx-coroutines-android:1.9.0")

    testImplementation("junit:junit:4.13.2")
    testImplementation("org.json:json:20240303")
}
