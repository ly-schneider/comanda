import com.android.build.api.variant.ResValue
import java.util.regex.Pattern
import org.jetbrains.kotlin.gradle.dsl.JvmTarget
import tenantbranding.GenerateTenantIconTask
import tenantbranding.initialsFor

fun semverFromTag(): Triple<Int, Int, Int> {
    val raw = (System.getenv("VERSION_TAG") ?: System.getenv("GITHUB_REF_NAME") ?: "")
    val tag = raw.removePrefix("v")
    val m = Pattern.compile("""^(\d+)\.(\d+)\.(\d+)(?:[-+].*)?$""").matcher(tag)
    return if (m.matches()) Triple(
        m.group(1).toInt(),
        m.group(2).toInt(),
        m.group(3).toInt()
    ) else Triple(0, 0, 1)
}

fun prop(name: String): String =
    ((project.findProperty(name) as String?) ?: System.getenv(name.uppercase())).orEmpty().trim()

// With tenantPkg set this builds one tenant's own app; without it, the neutral one.
val tenantPkg = prop("tenantPkg")
val tenantLabel = prop("appLabel")
val tenantIconSource = prop("iconSource")
val tenantIconBg = prop("iconBg").ifEmpty { "#1F1F1D" }
val isTenantBuild = tenantPkg.isNotEmpty()

if (isTenantBuild) {
    require(Regex("^t_[a-z0-9_]{1,60}$").matches(tenantPkg)) {
        "tenantPkg must match ^t_[a-z0-9_]{1,60}$ (the tenant's android_package_id), got \"$tenantPkg\""
    }
    require(tenantLabel.isNotEmpty()) { "appLabel is required for a tenant build" }
}

fun updateUrl(base: String, channel: String): String {
    if (base.isBlank()) return ""
    val sep = if (base.contains("?")) "&" else "?"
    var url = "$base${sep}channel=$channel"
    if (isTenantBuild) url += "&pkg=$tenantPkg"
    val token = prop("updateToken")
    if (token.isNotEmpty()) url += "&t=$token"
    return url
}

// The update feed already points at the API origin the PIN check needs. Returns
// "" when it cannot be derived confidently — callers building a shippable APK
// must treat that as fatal, since an empty base bricks the Device Setup PIN gate
// with no remote fix.
fun apiBaseUrl(updateCheckUrl: String): String {
    val explicit = prop("apiBaseUrl")
    if (explicit.isNotEmpty()) return explicit.trimEnd('/')
    if (!updateCheckUrl.contains("/v1/")) return ""
    return updateCheckUrl.substringBefore("/v1/").trimEnd('/')
}

val (maj, min, pat) = semverFromTag()
val computedVersionCode = maj * 10000 + min * 100 + pat
val computedVersionName = "$maj.$min.$pat"

plugins {
    alias(libs.plugins.android.application)
    alias(libs.plugins.kotlin.compose)
}

android {
    namespace = "ch.leys.comanda"
    compileSdk = 37

    defaultConfig {
        // From the tenant's immutable android_package_id — Android keys an install
        // off this, so a change reads as a different app: no upgrade, two icons.
        applicationId = if (isTenantBuild) "ch.leys.comanda.$tenantPkg" else "ch.leys.comanda"
        minSdk = 26
        targetSdk = 35
        versionName = computedVersionName
        versionCode = computedVersionCode

        testInstrumentationRunner = "androidx.test.runner.AndroidJUnitRunner"

        vectorDrawables { useSupportLibrary = true }

        // Generated icons use their own resource names; two res dirs declaring
        // ic_launcher in one source set is a duplicate-resource error.
        manifestPlaceholders["appIcon"] =
            if (isTenantBuild) "@mipmap/ic_launcher_tenant" else "@mipmap/ic_launcher"
        manifestPlaceholders["appRoundIcon"] =
            if (isTenantBuild) "@mipmap/ic_launcher_tenant_round" else "@mipmap/ic_launcher_round"

        buildConfigField("boolean", "TENANT_BUILD", "$isTenantBuild")
        buildConfigField("String", "TENANT_PACKAGE", "\"$tenantPkg\"")

        val sumupAffiliateKey = (project.findProperty("sumupAffiliateKey") as String?)
            ?: System.getenv("SUMUP_AFFILIATE_KEY")
            ?: ""
        if (gradle.startParameter.taskNames.any {
                it.contains(
                    "Release",
                    ignoreCase = true
                )
            } && sumupAffiliateKey.isBlank()) {
            throw GradleException("SUMUP_AFFILIATE_KEY (or -PsumupAffiliateKey) must be set for release builds")
        }
        buildConfigField("String", "SUMUP_AFFILIATE_KEY", "\"$sumupAffiliateKey\"")

        buildConfigField("String", "POS_URL", "\"http://localhost:3000/pos\"")
        buildConfigField("boolean", "DEV_BUILD", "true")
        buildConfigField("String", "UPDATE_CHECK_URL", "\"\"")
        // 10.0.2.2 is the host loopback from the emulator; debug and dev builds
        // talk to a locally running backend.
        buildConfigField("String", "API_BASE_URL", "\"http://10.0.2.2:8080\"")
    }

    buildFeatures {
        buildConfig = true
        resValues = true
    }

    // Configure release signing via environment variables (set by CI)
    signingConfigs {
        create("release") {
            val buildingRelease =
                gradle.startParameter.taskNames.any {
                    it.contains("Release", ignoreCase = true) || it.contains("Staging", ignoreCase = true)
                }

            val storeFilePath = (System.getenv("COMANDA_UPLOAD_STORE_FILE")
                ?: project.findProperty("COMANDA_UPLOAD_STORE_FILE") as String?) ?: ""
            val storePasswordVal = (System.getenv("COMANDA_UPLOAD_STORE_PASSWORD")
                ?: project.findProperty("COMANDA_UPLOAD_STORE_PASSWORD") as String?) ?: ""
            val keyAliasVal = (System.getenv("COMANDA_UPLOAD_KEY_ALIAS")
                ?: project.findProperty("COMANDA_UPLOAD_KEY_ALIAS") as String?) ?: ""
            val keyPasswordVal = (System.getenv("COMANDA_UPLOAD_KEY_PASSWORD")
                ?: project.findProperty("COMANDA_UPLOAD_KEY_PASSWORD") as String?) ?: ""

            val missing =
                storeFilePath.isBlank() || storePasswordVal.isBlank() || keyAliasVal.isBlank() || keyPasswordVal.isBlank()
            if (buildingRelease && missing) {
                throw GradleException("Release signing missing. Set COMANDA_UPLOAD_STORE_FILE, COMANDA_UPLOAD_STORE_PASSWORD, COMANDA_UPLOAD_KEY_ALIAS, COMANDA_UPLOAD_KEY_PASSWORD.")
            }

            if (storeFilePath.isNotBlank()) storeFile = file(storeFilePath)
            if (storePasswordVal.isNotBlank()) storePassword = storePasswordVal
            if (keyAliasVal.isNotBlank()) keyAlias = keyAliasVal
            if (keyPasswordVal.isNotBlank()) keyPassword = keyPasswordVal
        }
    }

    buildTypes {
        release {
            isMinifyEnabled = true
            proguardFiles(
                getDefaultProguardFile("proguard-android-optimize.txt"),
                "proguard-rules.pro"
            )

            signingConfig = signingConfigs.getByName("release")

            val buildingRelease =
                gradle.startParameter.taskNames.any { it.contains("Release", ignoreCase = true) }
            val posUrl = (project.findProperty("posUrl") as String?) ?: System.getenv("POS_URL")
            if (buildingRelease && (posUrl.isNullOrBlank())) {
                throw GradleException("POS_URL (or -PposUrl) must be set for release builds")
            }
            val posUrlRelease = posUrl ?: "https://example.com/pos"
            buildConfigField("String", "POS_URL", "\"$posUrlRelease\"")
            buildConfigField("boolean", "DEV_BUILD", "false")

            val updateCheckUrl = (project.findProperty("updateCheckUrl") as String?) ?: System.getenv("UPDATE_CHECK_URL") ?: ""
            val apiBase = apiBaseUrl(updateCheckUrl)
            if (buildingRelease && apiBase.isEmpty()) {
                throw GradleException("API_BASE_URL could not be derived. Set UPDATE_CHECK_URL (containing /v1/) or -PapiBaseUrl; without it Device Setup can never unlock.")
            }
            buildConfigField("String", "UPDATE_CHECK_URL", "\"${updateUrl(updateCheckUrl, "production")}\"")
            buildConfigField("String", "API_BASE_URL", "\"$apiBase\"")
        }

        create("dev") {
            initWith(getByName("debug"))
            isDebuggable = true
            signingConfig = signingConfigs.getByName("debug")
            applicationIdSuffix = ".dev"
            versionNameSuffix = "-dev"

            val posUrl = (project.findProperty("posUrl") as String?)
                ?: System.getenv("POS_URL")
                ?: "http://localhost:3000/pos"
            buildConfigField("String", "POS_URL", "\"$posUrl\"")
            buildConfigField("boolean", "DEV_BUILD", "true")

            val updateCheckUrl = (project.findProperty("updateCheckUrl") as String?) ?: System.getenv("UPDATE_CHECK_URL") ?: ""
            buildConfigField(
                "String",
                "API_BASE_URL",
                "\"${apiBaseUrl(updateCheckUrl).ifEmpty { "http://10.0.2.2:8080" }}\"",
            )
        }

        // Optional staging buildType for pre-prod testing
        create("staging") {
            initWith(getByName("release"))
            signingConfig = signingConfigs.getByName("release")
            applicationIdSuffix = ".staging"
            versionNameSuffix = "-staging"

            val posUrl = (project.findProperty("posUrl") as String?)
                ?: System.getenv("POS_URL")
                ?: "http://localhost:3000/pos"
            buildConfigField("String", "POS_URL", "\"$posUrl\"")
            buildConfigField("boolean", "DEV_BUILD", "false")

            val updateCheckUrl = (project.findProperty("updateCheckUrl") as String?) ?: System.getenv("UPDATE_CHECK_URL") ?: ""
            val apiBase = apiBaseUrl(updateCheckUrl)
            val buildingStaging =
                gradle.startParameter.taskNames.any { it.contains("Staging", ignoreCase = true) }
            if (buildingStaging && apiBase.isEmpty()) {
                throw GradleException("API_BASE_URL could not be derived. Set UPDATE_CHECK_URL (containing /v1/) or -PapiBaseUrl; without it Device Setup can never unlock.")
            }
            buildConfigField("String", "UPDATE_CHECK_URL", "\"${updateUrl(updateCheckUrl, "staging")}\"")
            buildConfigField("String", "API_BASE_URL", "\"$apiBase\"")
        }
    }
    compileOptions {
        sourceCompatibility = JavaVersion.VERSION_11
        targetCompatibility = JavaVersion.VERSION_11
        isCoreLibraryDesugaringEnabled = true
    }
    // Kotlin options moved to compilerOptions DSL below
    buildFeatures {
        compose = true
    }

    lint {
        // Vendor library (android-state) ships a lint registry without vendor metadata.
        // Do not fail release builds on such third-party lint issues.
        abortOnError = false
        checkReleaseBuilds = false
    }
}

// Always runs; produces nothing for the neutral build.
val generateTenantIcon = tasks.register<GenerateTenantIconTask>("generateTenantIcon") {
    generateIcon.set(isTenantBuild)
    initials.set(initialsFor(tenantLabel))
    backgroundColor.set(tenantIconBg)
    if (tenantIconSource.isNotEmpty()) {
        sourceImage.set(file(tenantIconSource))
    }
    outputDir.set(layout.buildDirectory.dir("generated/tenantIcon/res"))
}

androidComponents {
    onVariants { variant ->
        // Not per build type: the env tag has to survive the tenant's own name.
        val base = tenantLabel.ifEmpty { "Comanda POS" }
        val label = when (variant.buildType) {
            "dev" -> "$base (Dev)"
            "staging" -> "$base (Staging)"
            else -> base
        }
        variant.resValues.put(variant.makeResValueKey("string", "app_name"), ResValue(label))

        variant.sources.res?.addGeneratedSourceDirectory(
            generateTenantIcon,
            GenerateTenantIconTask::outputDir,
        )
    }
}

// Kotlin compiler options (KGP 2.x)
kotlin {
    compilerOptions {
        jvmTarget.set(JvmTarget.JVM_11)
    }
}

dependencies {
    implementation("com.sumup:merchant-sdk:7.1.0")
    coreLibraryDesugaring("com.android.tools:desugar_jdk_libs:2.1.5")
    implementation("com.github.DantSu:ESCPOS-ThermalPrinter-Android:3.4.0")
    implementation("com.google.zxing:core:3.5.4")
    implementation("com.journeyapps:zxing-android-embedded:4.3.0")
    implementation("androidx.webkit:webkit:1.17.0")

    implementation(libs.androidx.core.ktx)
    implementation(libs.androidx.lifecycle.runtime.ktx)
    implementation(libs.androidx.activity.compose)
    implementation(platform(libs.androidx.compose.bom))
    implementation(libs.androidx.ui)
    implementation(libs.androidx.ui.graphics)
    implementation(libs.androidx.ui.tooling.preview)
    implementation(libs.androidx.material3)
    testImplementation(libs.junit)
    androidTestImplementation(libs.androidx.junit)
    androidTestImplementation(libs.androidx.espresso.core)
    androidTestImplementation(platform(libs.androidx.compose.bom))
    androidTestImplementation(libs.androidx.ui.test.junit4)
    debugImplementation(libs.androidx.ui.tooling)
    debugImplementation(libs.androidx.ui.test.manifest)

    // Compile-only annotations used by transitive libraries to satisfy R8 analysis
    // Align with dependency constraints resolved for runtime classpath
    compileOnly("com.google.errorprone:error_prone_annotations:2.50.0")
}
