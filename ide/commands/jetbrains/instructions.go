package jetbrains

const JetbrainsManualInstructionsTemplate = `
Manual JetBrains IDE Setup Instructions:
=======================================

1. Close all JetBrains IDEs

2. Locate your IDE configuration directory:
   %s

   Examples:
   • IntelliJ IDEA: IntelliJIdea2023.3/idea.properties
   • PyCharm: PyCharm2023.3/idea.properties
   • WebStorm: WebStorm2023.3/idea.properties

3. Open or create the idea.properties file in a text editor

4. Add or modify the following line:
   idea.plugins.host=%s

5. Save the file and restart your IDE

Repository URL: %s

Supported IDEs: IntelliJ IDEA, PyCharm, WebStorm, PhpStorm, RubyMine, CLion, DataGrip, GoLand, Rider, Android Studio, AppCode, RustRover, Aqua
`
