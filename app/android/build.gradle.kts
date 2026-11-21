// Phone and web UI need arm64 only. The flutter tool always passes -Ptarget-platform
// (all ABIs for debug), so override it here before the Flutter plugin reads it.
// Exception: `flutter build apk --split-per-abi` (scripts/ci.py release build) passes
// -Psplit-per-abi=true and keeps every ABI, one APK each.
val splitPerAbi = findProperty("split-per-abi") == "true"
if (!splitPerAbi) {
    subprojects {
        setProperty("target-platform", "android-arm64")
        // Without this the Flutter plugin resets ndk.abiFilters to all ABIs (see app/build.gradle.kts).
        extra["disable-abi-filtering"] = "true"
    }
}

allprojects {
    repositories {
        google()
        mavenCentral()
    }
}

val newBuildDir: Directory =
    rootProject.layout.buildDirectory
        .dir("../../build")
        .get()
rootProject.layout.buildDirectory.value(newBuildDir)

subprojects {
    val newSubprojectBuildDir: Directory = newBuildDir.dir(project.name)
    project.layout.buildDirectory.value(newSubprojectBuildDir)
}
subprojects {
    project.evaluationDependsOn(":app")
}

tasks.register<Delete>("clean") {
    delete(rootProject.layout.buildDirectory)
}
