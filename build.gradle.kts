import com.fussionlabs.gradle.tasks.GoTask

plugins {
    alias(libs.plugins.go)
}

go {
    goVersion = "1.24.5"

    // disable default build targets
    os = listOf()
    arch = listOf()
}

val outFile = "build/mkv-ripper"
val goFiles = fileTree(project.projectDir) { include("**/*.go") }
val templFiles = fileTree("view") { include("**/*.templ") }
val templGoFiles = fileTree("view") { include("**/*.templ") }.map { it -> File(it.parent, it.name.replace(".templ", "_templ.go")) }

tasks.getByName("check").dependsOn("templGenerate")

tasks.getByName("assemble").dependsOn("mkv-ripper")
tasks.register("mkv-ripper", GoTask::class) {
    group = "build"
    description = "Compile the mkv-ripper binary"
    dependsOn("templGenerate")
    outputs.file(outFile)
    inputs.files(goFiles, templGoFiles)
    goTaskArgs = mutableListOf("build", "-o", outFile, "github.com/aravance/mkv-ripper/cmd/server")
}

tasks.register("templGenerate", GoTask::class) {
    group = "build"
    description = "Generate go code from .templ files"
    inputs.files(templFiles)
    outputs.files(templGoFiles)
    goTaskArgs = mutableListOf("tool", "github.com/a-h/templ/cmd/templ", "generate")
}

tasks.getByName("clean").doLast {
    fileTree("view") { include("**/*_templ.go") }
        .forEach { it -> it.delete() }
}
