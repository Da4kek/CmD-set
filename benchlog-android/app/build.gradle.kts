plugins {
    id("com.android.application")
    id("org.jetbrains.kotlin.android")
}

android {
    namespace = "com.benchlog"
    compileSdk = 34

    defaultConfig {
        applicationId = "com.benchlog"
        minSdk = 24
        targetSdk = 34
        versionCode = 1
        versionName = "0.1.0"

        ndk {
            abiFilters += "arm64-v8a"
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = false
            proguardFiles(getDefaultProguardFile("proguard-android-optimize.txt"))
        }
    }

    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_17
        targetCompatibility = JavaVersion.VERSION_17
    }

    kotlinOptions {
        jvmTarget = "17"
    }

    // Must be true: we need the .so files extracted to nativeLibraryDir at install time
    // so we can create symlinks to them and execute them from the terminal session.
    packaging {
        jniLibs { useLegacyPackaging = true }
    }
}

dependencies {
    implementation("com.github.termux.termux-app:terminal-emulator:v0.118.1")
    implementation("com.github.termux.termux-app:terminal-view:v0.118.1")
}
